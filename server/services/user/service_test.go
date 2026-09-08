package user

import (
	"context"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestService creates a user service with real dependencies for testing.
func setupTestService(t *testing.T) (*Service, *auth.UserManager, *storage.ProtoSQLStorage) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create user manager
	userManager := auth.NewUserManager(sqlStorage)

	// Create user service. Phone auth + bus are nil here: the default helper
	// covers the non-phone RPCs. AddPhoneNumber tests use setupPhoneTestService.
	service := New(userManager, sqlStorage, nil, nil)

	return service, userManager, sqlStorage
}

func TestNew(t *testing.T) {
	service, userManager, _ := setupTestService(t)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.userManager != userManager {
		t.Error("Expected userManager to be set correctly")
	}
}

// createAuthenticatedContext creates a context with authentication info for testing.
func createAuthenticatedContext(userID, email string, _ models.Role) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
	}
	return authn.SetInfo(context.Background(), authInfo)
}
