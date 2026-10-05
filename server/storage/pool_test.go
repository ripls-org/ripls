package storage

import (
	"database/sql"
	"testing"
)

// configurePool's limits are what the pool-saturation alert is measured
// against, so assert them rather than the call.
func TestConfigurePool(t *testing.T) {
	// sql.Open does not connect, so this needs no database.
	db, err := sql.Open("postgres", "postgres://unused/unused")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	configurePool(db)

	stats := db.Stats()
	if stats.MaxOpenConnections != maxOpenConns {
		t.Errorf("MaxOpenConnections = %d, want %d", stats.MaxOpenConnections, maxOpenConns)
	}
	if maxIdleConns > maxOpenConns {
		t.Errorf("maxIdleConns %d exceeds maxOpenConns %d; idle connections would be closed immediately",
			maxIdleConns, maxOpenConns)
	}
}

// Both pools this package opens must carry the limits, including the no-DDL
// path used by the read-only role.
func TestInitializedStorageUsesPoolLimits(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	t.Cleanup(cleanup)

	if got := sqlStorage.PoolStats().MaxOpenConnections; got != maxOpenConns {
		t.Errorf("initialized storage MaxOpenConnections = %d, want %d", got, maxOpenConns)
	}
}
