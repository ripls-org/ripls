# services/impact_metrics

The `impact_metrics` service implements the ImpactMetricsService RPC interface: community impact summaries (money saved, carbon prevented, time saved), user impact aggregates, metric detail breakdowns (time-series, rankings, comparisons), and methodology documentation.

## Key files

- `service.go` — service struct and constructor; wires `Calculator` and `MetricDetailCalculator` from `server/impact_metrics`.
- `metrics.go` — `GetCommunityImpactMetrics`, `GetUserImpactMetrics`.
- `metric_detail.go` — `GetCommunityMetricDetail`: returns breakdowns for a specific metric dimension.
- `actions.go` — `GetUserImpactActions`: surfaces undoable actions in the impact context.
- `leaderboard.go` — community leaderboard by impact.
- `percentile.go` — percentile computation helpers.
- `utilization.go` — gear utilization rate helpers.
- `time_to_solve.go` — time-to-solve metric helpers.
- `user_detail.go`, `user_impact_detail.go` — user-specific impact detail views.
- `user_metrics.go` — user-level metrics assembly.

## When to add code here vs. elsewhere

RPC handlers for impact data belong here. Calculation formulas and aggregation logic belong in `server/impact_metrics`. Per-item estimation belongs in `server/impact_metrics/estimator`. Community leaderboard discovery belongs in `server/services/leaderboard`.
