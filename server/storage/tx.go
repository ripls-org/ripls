package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.ripls.org/ripls/server/logging"
)

// WithTx runs fn inside a database transaction. The
// ProtoSQLStorage handed to fn routes every query through the
// transaction's connection — so any subsequent storage call
// (Insert, Update, Delete, DeleteByField, QueryByField, …)
// participates in the same atomic unit.
//
// Lifecycle:
//   - BeginTx is called with the supplied opts (pass nil for
//     read-committed, the default).
//   - The transaction Rollback is registered as a deferred
//     cleanup before fn runs. This makes the transaction
//     panic-safe — a panic in fn unwinds through the defer,
//     triggering Rollback.
//   - If fn returns nil, Commit fires. The deferred Rollback
//     then sees sql.ErrTxDone and is a no-op (Go's documented
//     idiom).
//   - If fn returns an error, the deferred Rollback fires and
//     the original error is returned. Rollback errors that
//     aren't sql.ErrTxDone are logged at WARN; we don't replace
//     the original error because diagnosing it matters more.
//
// Locked reload (re-check under lock) pattern:
//
// When an async writer does a read-modify-write on a row that a concurrent
// RPC can also modify (e.g. stock-image attach vs CancelRequest), wrap the
// entire sequence in WithTx and reload the row with
// QueryOptions{ForUpdate: true}. The SELECT FOR UPDATE acquires a row-level
// write lock that blocks any concurrent UPDATE on the same row until this
// transaction commits. After the locked reload, copy only the fields this
// writer owns onto the fresh snapshot, then write with plain tx.Update.
// Any concurrent change that committed before the lock is reflected on the
// fresh snapshot and is preserved on the merged write; any concurrent
// writer that arrives after the lock waits and then sees the committed state.
// server/jobs/community_purge.go (Safeguard #5) is the canonical example.
//
// This function does not retry. Callers that want retry-on-
// serialization-failure should layer that policy outside.
func (s *ProtoSQLStorage) WithTx(
	ctx context.Context,
	opts *sql.TxOptions,
	fn func(tx *ProtoSQLStorage) error,
) (err error) {
	rawTx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("BeginTx: %w", err)
	}

	// Always rollback on defer. After a successful Commit the
	// rollback returns sql.ErrTxDone, which we treat as benign
	// per database/sql convention. Any other error indicates a
	// driver-level issue (lost connection, etc.) and is logged
	// for visibility but does not replace the original error
	// path — fn's error, if any, is the actionable signal.
	defer func() {
		if rbErr := rawTx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logging.LoggerWithContext(ctx).WarnContext(ctx,
				"WithTx: rollback returned unexpected error",
				"error", rbErr,
			)
		}
	}()

	txStorage := s.withExecutor(NewInstrumentedTx(rawTx))

	if err := fn(txStorage); err != nil {
		return err
	}

	if err := rawTx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Run after-commit hooks on the caller's goroutine. Each hook is
	// expected to wrap any async work in logging.GoSafe per the goroutine-
	// safety convention (docs/server/architecture.md §"Goroutine safety").
	for _, fn := range txStorage.afterCommit {
		fn()
	}

	return nil
}

// withExecutor returns a shallow copy of s with exec replaced.
// All other fields (allowedTypes, dbSpec, embeddingConfigs,
// embedder, embeddingDone, db) are preserved by value/reference
// so the derived storage behaves identically to the parent
// outside of where queries get routed.
// afterCommit is reset to nil so each tx clone starts with a clean hook list.
func (s *ProtoSQLStorage) withExecutor(exec SQLExecutor) *ProtoSQLStorage {
	clone := *s
	clone.exec = exec
	clone.afterCommit = nil
	return &clone
}

// deferAfterCommit schedules fn to run after the surrounding transaction commits.
// If s is not inside a transaction (inTransaction() is false), fn is called
// immediately, preserving the pre-tx code path for types without array-column
// registrations.
func (s *ProtoSQLStorage) deferAfterCommit(fn func()) {
	if !s.inTransaction() {
		fn()
		return
	}
	s.afterCommit = append(s.afterCommit, fn)
}
