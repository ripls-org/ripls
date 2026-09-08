package auth

import (
	"context"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestRequireAuth(t *testing.T) {
	t.Run("authenticated context", func(t *testing.T) {
		authInfo := &Info{
			UserID: "user123",
			Email:  "test@example.com",
			Role:   models.Role_ROLE_USER,
		}

		ctx := authn.SetInfo(context.Background(), authInfo)

		retrievedInfo, err := RequireAuth(ctx)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if retrievedInfo.UserID != authInfo.UserID {
			t.Errorf("Expected UserID %s, got %s", authInfo.UserID, retrievedInfo.UserID)
		}

		if retrievedInfo.Email != authInfo.Email {
			t.Errorf("Expected Email %s, got %s", authInfo.Email, retrievedInfo.Email)
		}

		if retrievedInfo.Role != authInfo.Role {
			t.Errorf("Expected Role %v, got %v", authInfo.Role, retrievedInfo.Role)
		}
	})

	t.Run("unauthenticated context", func(t *testing.T) {
		ctx := context.Background()

		_, err := RequireAuth(ctx)
		if err == nil {
			t.Fatal("Expected authentication required error, got nil")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected CodeUnauthenticated, got %v", connectErr.Code())
		}
	})
}
