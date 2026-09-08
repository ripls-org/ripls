package statslog

import (
	"context"
	"database/sql"
	"runtime"
	"time"

	"go.ripls.org/ripls/server/logging"
)

// PoolStatsInterval is how often the DB-pool stats line is emitted. The
// monitoring dashboards and pool-saturation alert are tuned to this sampling
// rate.
const PoolStatsInterval = 15 * time.Second

// RuntimeStatsInterval is how often the Go-runtime stats line is emitted.
const RuntimeStatsInterval = 30 * time.Second

// StartPoolStatsLogger spawns a goroutine that logs PoolStatsMessage with
// PoolStatsAttrs every PoolStatsInterval until ctx is cancelled. stats is
// called on each tick (e.g. ProtoSQLStorage.PoolStats). The log line feeds
// the db_pool_* Cloud Monitoring log-based metrics so pool saturation can be
// alerted on independently of HTTP latency (#1613).
func StartPoolStatsLogger(ctx context.Context, logger *logging.Logger, stats func() sql.DBStats) {
	logging.GoSafe(ctx, "db-pool-stats-logger", func() {
		t := time.NewTicker(PoolStatsInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				logger.Info(PoolStatsMessage, PoolStatsAttrs(stats())...)
			}
		}
	})
}

// StartRuntimeStatsLogger spawns a goroutine that logs RuntimeStatsMessage
// with RuntimeStatsAttrs (goroutines, heap in-use, mean GC pause) every
// RuntimeStatsInterval until ctx is cancelled. The go_* log-based metrics
// extract these fields for the runtime dashboard and capacity planning
// (#1613).
func StartRuntimeStatsLogger(ctx context.Context, logger *logging.Logger) {
	logging.GoSafe(ctx, "runtime-stats-logger", func() {
		t := time.NewTicker(RuntimeStatsInterval)
		defer t.Stop()
		var prev runtime.MemStats
		runtime.ReadMemStats(&prev)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				var cur runtime.MemStats
				runtime.ReadMemStats(&cur)
				logger.Info(RuntimeStatsMessage, RuntimeStatsAttrs(prev, cur, runtime.NumGoroutine())...)
				prev = cur
			}
		}
	})
}
