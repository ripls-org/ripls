package admin

import (
	"testing"

	"go.ripls.org/ripls/server/storage"
)

func setupTestDatabase(t *testing.T) *storage.ProtoSQLStorage {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return db
}
