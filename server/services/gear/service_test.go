package gear

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// createAuthenticatedContext creates a context with authentication info.
func createAuthenticatedContext(userID, email string, role models.Role) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   role,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

// setupTestStorage creates a PostgreSQL database for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func TestNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(sqlStorage, mockBucket)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != sqlStorage {
		t.Error("Expected storage to be set correctly")
	}

	if service.bucket != mockBucket {
		t.Error("Expected bucket storage to be set correctly")
	}
}

func TestRequireAuth(t *testing.T) {
	t.Run("with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_ADMIN)

		authInfo, err := auth.RequireAuth(ctx)
		if err != nil {
			t.Fatalf("requireAuth failed: %v", err)
		}

		if authInfo.UserID != "user123" {
			t.Errorf("Expected UserID user123, got %s", authInfo.UserID)
		}

		if authInfo.Email != "test@example.com" {
			t.Errorf("Expected Email test@example.com, got %s", authInfo.Email)
		}

		if authInfo.Role != models.Role_ROLE_ADMIN {
			t.Errorf("Expected Role ROLE_ADMIN, got %v", authInfo.Role)
		}
	})

	t.Run("without authentication", func(t *testing.T) {
		ctx := context.Background()

		_, err := auth.RequireAuth(ctx)
		if err == nil {
			t.Fatal("Expected error for unauthenticated context")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})
}

func TestService_MultipleGearOperations(t *testing.T) {
	storage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(storage, mockBucket)

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user123",
		Email: "test@example.com",
		Name:  "Test User 123",
	}
	_, err := storage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Add multiple gear items
	gearItems := []struct {
		Name        string
		Description string
	}{
		{Name: "Drill", Description: "Power drill"},
		{Name: "Saw", Description: "Circular saw"},
		{Name: "Hammer", Description: "Claw hammer"},
	}

	var addedIDs []string

	// Add all gear items
	for i, gear := range gearItems {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String(gear.Name),
			Description: proto.String(gear.Description),
		})
		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("Failed to add gear %d: %v", i, err)
		}
		addedIDs = append(addedIDs, resp.Msg.Id)
	}

	// Retrieve and verify each gear item
	for i, expectedGear := range gearItems {
		getReq := connect.NewRequest(&api.GetGearRequest{Id: addedIDs[i]})
		getResp, err := service.GetGear(ctx, getReq)
		if err != nil {
			t.Fatalf("Failed to get gear %d: %v", i, err)
		}

		if getResp.Msg.Name != expectedGear.Name {
			t.Errorf("Gear %d name mismatch: expected %s, got %s",
				i, expectedGear.Name, getResp.Msg.Name)
		}
		if getResp.Msg.Description != expectedGear.Description {
			t.Errorf("Gear %d description mismatch: expected %s, got %s",
				i, expectedGear.Description, getResp.Msg.Description)
		}
		if getResp.Msg.Owner == nil {
			t.Fatalf("Gear %d Owner is nil", i)
		}
		if getResp.Msg.Owner.Id != "user123" {
			t.Errorf("Gear %d owner mismatch: expected user123, got %s",
				i, getResp.Msg.Owner.Id)
		}
	}

	// Verify all IDs are unique
	idSet := make(map[string]bool)
	for _, id := range addedIDs {
		if idSet[id] {
			t.Errorf("Duplicate ID found: %s", id)
		}
		idSet[id] = true
	}
}
