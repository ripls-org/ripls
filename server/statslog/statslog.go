package statslog

import (
	"database/sql"
	"runtime"
)

// PoolStatsMessage is the log message of the periodic DB-pool stats line.
// The db_pool_* log-based metric filters match it exactly.
const PoolStatsMessage = "db pool stats"

// RuntimeStatsMessage is the log message of the periodic Go-runtime stats
// line. The go_* log-based metric filters match it exactly.
const RuntimeStatsMessage = "go runtime stats"

// PoolStatsAttrs converts a sql.DBStats snapshot into the structured log
// attributes the db_pool_* log-based metrics extract. db_pool_utilization is
// InUse/MaxOpenConnections precomputed server-side so the pool-saturation
// alert is a single-metric threshold rather than a cross-metric ratio.
//
// The denominator is the pool's limit, not the connections open right now.
// Go's pool opens connections on demand, so an idle server sits at one open
// connection, and a single in-flight query against it reads as fully utilized
// — which is how the saturation alert fired on a pool using one of its ten
// connections. Against the limit, that is 0.1, and 0.9 means what the alert
// says: the pool is nearly out of connections. It is 0 for an unlimited pool
// (MaxOpenConnections <= 0), where there is no capacity to be short of.
func PoolStatsAttrs(stats sql.DBStats) []any {
	utilization := 0.0
	if stats.MaxOpenConnections > 0 {
		utilization = float64(stats.InUse) / float64(stats.MaxOpenConnections)
	}
	return []any{
		"db_pool_open", stats.OpenConnections,
		"db_pool_max_open", stats.MaxOpenConnections,
		"db_pool_in_use", stats.InUse,
		"db_pool_idle", stats.Idle,
		"db_pool_wait_count", stats.WaitCount,
		"db_pool_wait_seconds", stats.WaitDuration.Seconds(),
		"db_pool_utilization", utilization,
	}
}

// RuntimeStatsAttrs builds the structured log attributes the go_* log-based
// metrics extract, from two consecutive MemStats snapshots and the current
// goroutine count. gc_pause_ms is the mean GC pause between the snapshots,
// or 0 when no GC cycle completed in the interval.
func RuntimeStatsAttrs(prev, cur runtime.MemStats, goroutines int) []any {
	var pauseMs float64
	if cycles := cur.NumGC - prev.NumGC; cycles > 0 {
		pauseMs = float64(cur.PauseTotalNs-prev.PauseTotalNs) / float64(cycles) / 1e6
	}
	return []any{
		"go_goroutines", goroutines,
		"heap_inuse_bytes", cur.HeapInuse,
		"gc_pause_ms", pauseMs,
	}
}
