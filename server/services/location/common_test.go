package location

import (
	"context"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/storage"
)

// setupTestService creates a location service with test dependencies.
func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage, storage.BucketStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create local bucket storage for testing
	tmpDir := t.TempDir()
	bucketStorage, err := storage.NewLocalBucketStorage(tmpDir+"/media", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to initialize bucket storage: %v", err)
	}

	// Create a location provider backed by the Mapbox client (geocoding
	// tests use a mock with custom transport).
	provider := location.NewMapboxClient("test-access-token")

	// Create location service
	service := New(sqlStorage, bucketStorage, provider)

	return service, sqlStorage, bucketStorage
}

// createAuthContext creates a context with authentication info for testing.
func createAuthContext(userID, email string) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

func TestNew(t *testing.T) {
	service, sqlStorage, bucketStorage := setupTestService(t)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != sqlStorage {
		t.Error("Expected storage to be set correctly")
	}

	if service.bucket != bucketStorage {
		t.Error("Expected bucket to be set correctly")
	}

	if service.locationProvider == nil {
		t.Error("Expected locationProvider to be set")
	}
}
