package esm

import (
	"testing"

	"go.ripls.org/ripls/server/storage"
)

// setupTestStorage creates an isolated PostgreSQL test database via
// testcontainers and registers cleanup with the test.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}
