// Per-request query counting and timing via context, for profiling storage layer efficiency.
//
// SQLDB is the interface used by all storage code. InstrumentedDB wraps *sql.DB
// so every QueryContext, ExecContext, and QueryRowContext call automatically
// increments the per-request counter stored in the context by WithQueryStats.

package storage

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// SQLExecutor is the narrowest database interface needed to issue
// queries. Both *sql.DB and *sql.Tx satisfy it (and so do their
// InstrumentedDB / InstrumentedTx wrappers). ProtoSQLStorage
// holds an SQLExecutor for query routing — when running inside a
// transaction, that field points at an InstrumentedTx so every
// storage call participates in the tx.
type SQLExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLDB is the full database interface used by ProtoSQLStorage —
// SQLExecutor plus connection-lifetime methods. *sql.DB satisfies
// it directly; InstrumentedDB adds query counting.
type SQLDB interface {
	SQLExecutor
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	Close() error
	PingContext(ctx context.Context) error
	Stats() sql.DBStats
}

// queryStatsKey is the context key for per-request query statistics.
type queryStatsKey struct{}

// QueryStats tracks the number and total duration of database queries within a request.
type QueryStats struct {
	Count   atomic.Int64
	TotalNs atomic.Int64 // total query time in nanoseconds
}

// TotalDuration returns the total query duration as a time.Duration.
func (s *QueryStats) TotalDuration() time.Duration {
	return time.Duration(s.TotalNs.Load())
}

// WithQueryStats attaches a new QueryStats to the context for per-request tracking.
func WithQueryStats(ctx context.Context) context.Context {
	return context.WithValue(ctx, queryStatsKey{}, &QueryStats{})
}

// GetQueryStats retrieves the QueryStats from the context, or nil if not set.
func GetQueryStats(ctx context.Context) *QueryStats {
	stats, _ := ctx.Value(queryStatsKey{}).(*QueryStats)
	return stats
}

// slowQueryThreshold is the duration above which a query is logged at WARN level.
const slowQueryThreshold = 100 * time.Millisecond

// maxQueryLogLen is the maximum length of a query string in slow query log messages.
const maxQueryLogLen = 200

// recordQuery increments the query counter and adds the duration to the
// context stats. Queries exceeding slowQueryThreshold are logged at WARN
// level. Safe to call even if ctx has no QueryStats (no-op for stats in
// that case).
func recordQuery(ctx context.Context, query string, duration time.Duration) {
	if stats := GetQueryStats(ctx); stats != nil {
		stats.Count.Add(1)
		stats.TotalNs.Add(int64(duration))
	}
	if duration >= slowQueryThreshold {
		truncated := query
		if len(truncated) > maxQueryLogLen {
			truncated = truncated[:maxQueryLogLen] + "..."
		}
		// query_kind feeds the db_slow_query_duration log-based metric (#1613).
		slog.WarnContext(ctx, "slow query",
			"duration_ms", duration.Milliseconds(),
			"query_kind", queryKind(query),
			"query", truncated,
		)
	}
}

// queryKind returns the leading SQL keyword (lowercased) of a query, mapped
// to one of select, insert, update, delete, or other. Used as a bounded label
// on the DB query duration histogram.
func queryKind(query string) string {
	q := strings.TrimLeft(query, " \t\n\r(")
	if len(q) < 6 {
		return "other"
	}
	switch strings.ToLower(q[:6]) {
	case "select":
		return "select"
	case "insert":
		return "insert"
	case "update":
		return "update"
	case "delete":
		return "delete"
	}
	return "other"
}

// InstrumentedDB wraps *sql.DB and records every query in the context's QueryStats.
type InstrumentedDB struct {
	*sql.DB
}

// NewInstrumentedDB wraps a *sql.DB with automatic query counting.
func NewInstrumentedDB(db *sql.DB) *InstrumentedDB {
	return &InstrumentedDB{DB: db}
}

// QueryContext executes a query and records its duration in the context stats.
func (d *InstrumentedDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := d.DB.QueryContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return rows, err
}

// ExecContext executes a statement and records its duration in the context stats.
func (d *InstrumentedDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := d.DB.ExecContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return result, err
}

// QueryRowContext executes a single-row query and records its duration in the context stats.
func (d *InstrumentedDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := d.DB.QueryRowContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return row
}

// InstrumentedTx wraps *sql.Tx with the same per-query stats
// recording as InstrumentedDB. Returned by ProtoSQLStorage's
// WithTx so transactional queries flow through the same
// instrumentation as non-transactional ones.
type InstrumentedTx struct {
	*sql.Tx
}

// NewInstrumentedTx wraps a *sql.Tx with automatic query counting.
func NewInstrumentedTx(tx *sql.Tx) *InstrumentedTx {
	return &InstrumentedTx{Tx: tx}
}

// QueryContext executes a query inside the transaction and records its duration.
func (t *InstrumentedTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := t.Tx.QueryContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return rows, err
}

// ExecContext executes a statement inside the transaction and records its duration.
func (t *InstrumentedTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := t.Tx.ExecContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return result, err
}

// QueryRowContext executes a single-row query inside the transaction and records its duration.
func (t *InstrumentedTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := t.Tx.QueryRowContext(ctx, query, args...)
	recordQuery(ctx, query, time.Since(start))
	return row
}
