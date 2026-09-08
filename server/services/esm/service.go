package esm

import (
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the ESMService RPC interface.
type Service struct {
	storage *storage.ProtoSQLStorage
}

// New creates a new ESM service.
func New(sqlStorage *storage.ProtoSQLStorage) *Service {
	return &Service{
		storage: sqlStorage,
	}
}

// Ensure Service implements ESMServiceHandler.
var _ apiconnect.ESMServiceHandler = (*Service)(nil)
