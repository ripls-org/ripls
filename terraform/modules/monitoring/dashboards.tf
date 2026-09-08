# =============================================================================
# SLI/SLO DASHBOARDS (issue #1613)
# =============================================================================
# Four dashboards over the log-based metrics in log_metrics.tf, mirroring the
# widget inventory of the abandoned Prometheus attempt (PR #1612) but using
# Cloud Monitoring filters + percentile aggregations instead of PromQL.
#
# Aggregation pattern for DISTRIBUTION metrics: ALIGN_DELTA per series (keeps
# the distribution), then REDUCE_PERCENTILE_XX across series — this merges
# distributions across Cloud Run instances/revisions before computing the
# percentile. Gauge-like sampled metrics (pool, runtime) instead use
# ALIGN_PERCENTILE_99 + REDUCE_MAX, which approximates the worst instance.
#
# These dashboards used to show as "update in-place" on *every* plan, applied
# as a no-op, and come straight back. The cause was source that did not match
# what the Monitoring API stores, in two specific ways — the API adds
# `targetAxis: "Y1"` to every dataSet, and it drops `xPos`/`yPos` when they are
# zero, since zero is the proto3 default and is not serialized. Writing the
# source in the API's own normal form is what makes the diff go away: hence the
# explicit `targetAxis` on every dataSet below, and no `xPos = 0` / `yPos = 0`
# (a tile with neither still lands at the origin).
#
# Keep new widgets in that form. A dataSet without `targetAxis`, or a tile that
# spells out a zero position, re-opens the permanent diff.
#
# Still do NOT ignore_changes on dashboard_json — that would stop real edits
# from deploying, which is a far worse trade than a noisy plan ever was.

locals {
  # SLO targets pinned in widget titles so a dashboard reader sees the target
  # next to the line. The same variables drive the alert policies in
  # alerts.tf, so titles and alerts cannot drift apart.
  slo_targets = {
    rpc_p95_latency_ms          = var.rpc_p95_latency_threshold_ms
    rpc_error_rate_percent      = var.rpc_error_rate_threshold * 100
    db_pool_utilization_percent = var.db_pool_saturation_threshold * 100
  }

  # Monitoring filter for a log-based user metric (dashboards use space-
  # separated conjunction).
  lbm_filter = {
    rpc_requests           = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_requests.name}\" resource.type=\"cloud_run_revision\""
    rpc_request_duration   = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_request_duration.name}\" resource.type=\"cloud_run_revision\""
    db_request_duration    = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_request_duration.name}\" resource.type=\"cloud_run_revision\""
    db_slow_queries        = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_slow_queries.name}\" resource.type=\"cloud_run_revision\""
    db_slow_query_duration = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_slow_query_duration.name}\" resource.type=\"cloud_run_revision\""
    db_pool_open           = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool["open"].name}\" resource.type=\"cloud_run_revision\""
    db_pool_in_use         = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool["in_use"].name}\" resource.type=\"cloud_run_revision\""
    db_pool_idle           = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool["idle"].name}\" resource.type=\"cloud_run_revision\""
    db_pool_utilization    = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool_utilization.name}\" resource.type=\"cloud_run_revision\""
    go_goroutines          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.go_goroutines.name}\" resource.type=\"cloud_run_revision\""
    go_heap_inuse_bytes    = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.go_heap_inuse_bytes.name}\" resource.type=\"cloud_run_revision\""
    go_gc_pause_ms         = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.go_gc_pause_ms.name}\" resource.type=\"cloud_run_revision\""
  }

  # Reusable aggregation shapes.
  agg_distribution_percentile = {
    p50 = {
      alignmentPeriod    = "300s"
      perSeriesAligner   = "ALIGN_DELTA"
      crossSeriesReducer = "REDUCE_PERCENTILE_50"
      groupByFields      = ["metric.label.\"rpc_method\""]
    }
    p95 = {
      alignmentPeriod    = "300s"
      perSeriesAligner   = "ALIGN_DELTA"
      crossSeriesReducer = "REDUCE_PERCENTILE_95"
      groupByFields      = ["metric.label.\"rpc_method\""]
    }
    p99 = {
      alignmentPeriod    = "300s"
      perSeriesAligner   = "ALIGN_DELTA"
      crossSeriesReducer = "REDUCE_PERCENTILE_99"
      groupByFields      = ["metric.label.\"rpc_method\""]
    }
  }

  # No groupByFields: these are un-grouped gauges, and an empty list is the
  # default the API does not serialize — spelling it out re-opens a permanent
  # plan diff (see the header note).
  agg_gauge_worst_instance = {
    alignmentPeriod    = "300s"
    perSeriesAligner   = "ALIGN_PERCENTILE_99"
    crossSeriesReducer = "REDUCE_MAX"
  }
}

# -----------------------------------------------------------------------------
# 1. Per-RPC latency (P50/P95/P99)
# -----------------------------------------------------------------------------
resource "google_monitoring_dashboard" "rpc_latency" {
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "RPC Latency - ${var.service_name}"
    mosaicLayout = {
      columns = 12
      tiles = [
        {
          width  = 12
          height = 4
          widget = {
            title = "P95 latency by RPC (ms) — SLO ≤ ${local.slo_targets.rpc_p95_latency_ms}ms, non-streaming"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.rpc_request_duration
                    aggregation = local.agg_distribution_percentile.p95
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
        {
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "P50 latency by RPC (ms)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.rpc_request_duration
                    aggregation = local.agg_distribution_percentile.p50
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
        {
          xPos   = 6
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "P99 latency by RPC (ms)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.rpc_request_duration
                    aggregation = local.agg_distribution_percentile.p99
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
      ]
    }
  })
}

# -----------------------------------------------------------------------------
# 2. Per-RPC traffic & error rate
# -----------------------------------------------------------------------------
resource "google_monitoring_dashboard" "rpc_errors" {
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "RPC Traffic & Errors - ${var.service_name}"
    mosaicLayout = {
      columns = 12
      tiles = [
        {
          width  = 12
          height = 4
          widget = {
            title = "5xx error fraction by RPC — SLO ≤ ${local.slo_targets.rpc_error_rate_percent}%"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilterRatio = {
                    numerator = {
                      filter = "${local.lbm_filter.rpc_requests} metric.label.\"status\"=monitoring.regex.full_match(\"5..\")"
                      aggregation = {
                        alignmentPeriod    = "300s"
                        perSeriesAligner   = "ALIGN_RATE"
                        crossSeriesReducer = "REDUCE_SUM"
                        groupByFields      = ["metric.label.\"rpc_method\""]
                      }
                    }
                    denominator = {
                      filter = local.lbm_filter.rpc_requests
                      aggregation = {
                        alignmentPeriod    = "300s"
                        perSeriesAligner   = "ALIGN_RATE"
                        crossSeriesReducer = "REDUCE_SUM"
                        groupByFields      = ["metric.label.\"rpc_method\""]
                      }
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "fraction" }
            }
          }
        },
        {
          yPos   = 4
          width  = 12
          height = 4
          widget = {
            title = "Request rate by RPC (req/s)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = local.lbm_filter.rpc_requests
                    aggregation = {
                      alignmentPeriod    = "300s"
                      perSeriesAligner   = "ALIGN_RATE"
                      crossSeriesReducer = "REDUCE_SUM"
                      groupByFields      = ["metric.label.\"rpc_method\""]
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "req/s" }
            }
          }
        },
        {
          yPos   = 8
          width  = 6
          height = 4
          widget = {
            title = "5xx rate by RPC (errors/s)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "${local.lbm_filter.rpc_requests} metric.label.\"status\"=monitoring.regex.full_match(\"5..\")"
                    aggregation = {
                      alignmentPeriod    = "300s"
                      perSeriesAligner   = "ALIGN_RATE"
                      crossSeriesReducer = "REDUCE_SUM"
                      groupByFields      = ["metric.label.\"rpc_method\""]
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "errors/s" }
            }
          }
        },
        {
          xPos   = 6
          yPos   = 8
          width  = 6
          height = 4
          widget = {
            title = "4xx rate by RPC (informational — client errors)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = "${local.lbm_filter.rpc_requests} metric.label.\"status\"=monitoring.regex.full_match(\"4..\")"
                    aggregation = {
                      alignmentPeriod    = "300s"
                      perSeriesAligner   = "ALIGN_RATE"
                      crossSeriesReducer = "REDUCE_SUM"
                      groupByFields      = ["metric.label.\"rpc_method\""]
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "errors/s" }
            }
          }
        },
      ]
    }
  })
}

# -----------------------------------------------------------------------------
# 3. DB health
# -----------------------------------------------------------------------------
resource "google_monitoring_dashboard" "db_health" {
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "DB Health - ${var.service_name}"
    mosaicLayout = {
      columns = 12
      tiles = [
        {
          width  = 12
          height = 4
          widget = {
            title = "P95 DB time per request by RPC (ms) — uncensored, all queries"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.db_request_duration
                    aggregation = local.agg_distribution_percentile.p95
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
        {
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "Pool connections (worst instance): open / in-use / idle"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter      = local.lbm_filter.db_pool_open
                      aggregation = local.agg_gauge_worst_instance
                    }
                  }
                  plotType       = "LINE"
                  targetAxis     = "Y1"
                  legendTemplate = "open"
                },
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter      = local.lbm_filter.db_pool_in_use
                      aggregation = local.agg_gauge_worst_instance
                    }
                  }
                  plotType       = "LINE"
                  targetAxis     = "Y1"
                  legendTemplate = "in_use"
                },
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter      = local.lbm_filter.db_pool_idle
                      aggregation = local.agg_gauge_worst_instance
                    }
                  }
                  plotType       = "LINE"
                  targetAxis     = "Y1"
                  legendTemplate = "idle"
                },
              ]
              yAxis = { scale = "LINEAR", label = "connections" }
            }
          }
        },
        {
          xPos   = 6
          yPos   = 4
          width  = 6
          height = 4
          widget = {
            title = "Pool utilization (worst instance) — SLO ≤ ${local.slo_targets.db_pool_utilization_percent}%"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.db_pool_utilization
                    aggregation = local.agg_gauge_worst_instance
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "in_use / open" }
            }
          }
        },
        {
          yPos   = 8
          width  = 6
          height = 4
          widget = {
            title = "Slow-query rate by kind (queries ≥ 100ms per second)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = local.lbm_filter.db_slow_queries
                    aggregation = {
                      alignmentPeriod    = "300s"
                      perSeriesAligner   = "ALIGN_RATE"
                      crossSeriesReducer = "REDUCE_SUM"
                      groupByFields      = ["metric.label.\"query_kind\""]
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "queries/s" }
            }
          }
        },
        {
          xPos   = 6
          yPos   = 8
          width  = 6
          height = 4
          widget = {
            title = "P95 slow-query duration by kind (ms) — censored below 100ms"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter = local.lbm_filter.db_slow_query_duration
                    aggregation = {
                      alignmentPeriod    = "300s"
                      perSeriesAligner   = "ALIGN_DELTA"
                      crossSeriesReducer = "REDUCE_PERCENTILE_95"
                      groupByFields      = ["metric.label.\"query_kind\""]
                    }
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
      ]
    }
  })
}

# -----------------------------------------------------------------------------
# 4. Go runtime
# -----------------------------------------------------------------------------
resource "google_monitoring_dashboard" "go_runtime" {
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "Go Runtime - ${var.service_name}"
    mosaicLayout = {
      columns = 12
      tiles = [
        {
          width  = 6
          height = 4
          widget = {
            title = "Goroutines (worst instance) — sustained climb = leak"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.go_goroutines
                    aggregation = local.agg_gauge_worst_instance
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "goroutines" }
            }
          }
        },
        {
          xPos   = 6
          width  = 6
          height = 4
          widget = {
            title = "Heap in-use (worst instance) — watch vs Cloud Run memory limit"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.go_heap_inuse_bytes
                    aggregation = local.agg_gauge_worst_instance
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "bytes" }
            }
          }
        },
        {
          yPos   = 4
          width  = 12
          height = 4
          widget = {
            title = "Mean GC pause between samples (ms)"
            xyChart = {
              dataSets = [{
                timeSeriesQuery = {
                  timeSeriesFilter = {
                    filter      = local.lbm_filter.go_gc_pause_ms
                    aggregation = local.agg_gauge_worst_instance
                  }
                }
                plotType   = "LINE"
                targetAxis = "Y1"
              }]
              yAxis = { scale = "LINEAR", label = "ms" }
            }
          }
        },
      ]
    }
  })
}
