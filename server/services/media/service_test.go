package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

var (
	// testJPEGBytes is a real, decodable JPEG — StoreMedia now sanitizes
	// (decodes + re-encodes) user images, so a magic-byte stub no longer works.
	testJPEGBytes = makeTestJPEG()
	// testMP4Bytes is a video stub; video is not sanitized, so the stub is fine.
	testMP4Bytes = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
)

// makeTestJPEG returns a small, valid JPEG that passes both content validation
// and sanitization (which decodes and re-encodes it).
func makeTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

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
