package statslog

import (
	"database/sql"
	"runtime"
	"testing"
	"time"
)

// attrsToMap converts the alternating key/value slice produced by the attr
// builders into a map for assertion.
func attrsToMap(t *testing.T, attrs []any) map[string]any {
	t.Helper()
	if len(attrs)%2 != 0 {
		t.Fatalf("attrs has odd length %d", len(attrs))
	}
	m := make(map[string]any, len(attrs)/2)
	for i := 0; i < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			t.Fatalf("attr key at %d is %T, want string", i, attrs[i])
		}
		m[key] = attrs[i+1]
	}
	return m
}

func TestPoolStatsAttrs(t *testing.T) {
	attrs := attrsToMap(t, PoolStatsAttrs(sql.DBStats{
		MaxOpenConnections: 10,
		OpenConnections:    10,
		InUse:              9,
		Idle:               1,
		WaitCount:          42,
		WaitDuration:       1500 * time.Millisecond,
	}))

	if got := attrs["db_pool_open"]; got != 10 {
		t.Errorf("db_pool_open = %v, want 10", got)
	}
	if got := attrs["db_pool_max_open"]; got != 10 {
		t.Errorf("db_pool_max_open = %v, want 10", got)
	}
	if got := attrs["db_pool_in_use"]; got != 9 {
		t.Errorf("db_pool_in_use = %v, want 9", got)
	}
	if got := attrs["db_pool_idle"]; got != 1 {
		t.Errorf("db_pool_idle = %v, want 1", got)
	}
	if got := attrs["db_pool_wait_count"]; got != int64(42) {
		t.Errorf("db_pool_wait_count = %v, want 42", got)
	}
	if got := attrs["db_pool_wait_seconds"]; got != 1.5 {
		t.Errorf("db_pool_wait_seconds = %v, want 1.5", got)
	}
	if got := attrs["db_pool_utilization"]; got != 0.9 {
		t.Errorf("db_pool_utilization = %v, want 0.9", got)
	}
}

func TestPoolStatsAttrs_UnlimitedPool(t *testing.T) {
	attrs := attrsToMap(t, PoolStatsAttrs(sql.DBStats{OpenConnections: 3, InUse: 3}))
	if got := attrs["db_pool_utilization"]; got != 0.0 {
		t.Errorf("db_pool_utilization = %v, want 0 for an unlimited pool", got)
	}
}

// The shape that fired the saturation alert on an idle server: the pool's one
// open connection busy, nine of its ten still available.
func TestPoolStatsAttrs_BusyLoneConnectionIsNotSaturated(t *testing.T) {
	attrs := attrsToMap(t, PoolStatsAttrs(sql.DBStats{
		MaxOpenConnections: 10,
		OpenConnections:    1,
		InUse:              1,
	}))
	if got := attrs["db_pool_utilization"]; got != 0.1 {
		t.Errorf("db_pool_utilization = %v, want 0.1 (1 of 10), not 1.0 (1 of 1)", got)
	}
}

func TestPoolStatsAttrs_Saturated(t *testing.T) {
	attrs := attrsToMap(t, PoolStatsAttrs(sql.DBStats{
		MaxOpenConnections: 10,
		OpenConnections:    10,
		InUse:              10,
		WaitCount:          7,
	}))
	if got := attrs["db_pool_utilization"]; got != 1.0 {
		t.Errorf("db_pool_utilization = %v, want 1.0 when every connection is in use", got)
	}
}

func TestRuntimeStatsAttrs_WithGC(t *testing.T) {
	prev := runtime.MemStats{NumGC: 10, PauseTotalNs: 1_000_000}
	cur := runtime.MemStats{NumGC: 14, PauseTotalNs: 9_000_000, HeapInuse: 32 << 20}

	attrs := attrsToMap(t, RuntimeStatsAttrs(prev, cur, 123))

	if got := attrs["go_goroutines"]; got != 123 {
		t.Errorf("go_goroutines = %v, want 123", got)
	}
	if got := attrs["heap_inuse_bytes"]; got != uint64(32<<20) {
		t.Errorf("heap_inuse_bytes = %v, want %d", got, uint64(32<<20))
	}
	// 8ms of pause across 4 cycles = 2ms mean.
	if got := attrs["gc_pause_ms"]; got != 2.0 {
		t.Errorf("gc_pause_ms = %v, want 2.0", got)
	}
}

func TestRuntimeStatsAttrs_NoGC(t *testing.T) {
	snap := runtime.MemStats{NumGC: 10, PauseTotalNs: 1_000_000}

	attrs := attrsToMap(t, RuntimeStatsAttrs(snap, snap, 1))

	if got := attrs["gc_pause_ms"]; got != 0.0 {
		t.Errorf("gc_pause_ms = %v, want 0 when no GC cycle completed", got)
	}
}
