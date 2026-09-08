# statslog

The periodic stats log lines that feed Cloud Monitoring log-based metrics
(#1613): DB connection-pool stats every 15s and Go runtime stats every 30s.
The package owns both the pure, testable field construction and the ticker
goroutines that emit the lines (started from `server/main.go` at boot).

## Key files

- `statslog.go` — `PoolStatsAttrs` (from `sql.DBStats`, including precomputed
  `db_pool_utilization` for the pool-saturation alert) and
  `RuntimeStatsAttrs` (goroutines, heap in-use, mean GC pause between
  snapshots), plus the `PoolStatsMessage` / `RuntimeStatsMessage` log-message
  constants the Terraform metric filters match.
- `tickers.go` — `StartPoolStatsLogger` / `StartRuntimeStatsLogger`: panic-safe
  goroutines emitting the lines on the `PoolStatsInterval` / `RuntimeStatsInterval`
  cadence the dashboards are tuned to.

## When to add code here vs. elsewhere

Add to this package when a new periodic stats line needs structured fields
extracted by a log-based metric in `terraform/modules/monitoring/log_metrics.tf`
— keep the builder pure (inputs → `[]any` attrs) so it stays unit-testable,
and give it a `Start*Logger` ticker here. Per-request fields belong in
`server/middleware/` (logging, query-stats).
