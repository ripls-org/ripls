package admin

import (
	"go.ripls.org/ripls/server/storage"
)

// Service handles administrative operations.
type Service struct {
	db      *storage.ProtoSQLStorage
	devMode bool
}

// NewService creates a new Service instance.
// When devMode is true, destructive operations like ResetDatabase are enabled.
func NewService(db *storage.ProtoSQLStorage, devMode bool) *Service {
	return &Service{db: db, devMode: devMode}
}
