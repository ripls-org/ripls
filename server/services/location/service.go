package location

import (
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the LocationService RPC interface.
type Service struct {
	storage          *storage.ProtoSQLStorage
	bucket           storage.BucketStorage
	locationProvider location.Provider
}

// New creates a new location service.
func New(sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, provider location.Provider) *Service {
	return &Service{
		storage:          sqlStorage,
		bucket:           bucketStorage,
		locationProvider: provider,
	}
}

// Verify that Service implements the LocationServiceHandler interface.
var _ apiconnect.LocationServiceHandler = (*Service)(nil)
