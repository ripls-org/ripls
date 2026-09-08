package login

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestService creates a login service with real dependencies for testing.
func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage, *auth.UserManager, *auth.TokenConfig) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create local bucket storage for testing
	tmpDir := t.TempDir()
	bucketStorage, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	// Create test token config
	authTokenConfig := &auth.TokenConfig{
		Secret:     []byte("test-secret-key"),
		Expiration: time.Hour,
	}

	// Create user manager
	userManager := auth.NewUserManager(sqlStorage)

	// Create OIDC manager (empty for tests)
	oidcManager := auth.NewOIDCProviderManager()

	// Create mock email service
	emailService := &email.MockEmailService{}

	service := New(Config{
		UserManager:            userManager,
		AuthTokenConfig:        authTokenConfig,
		OIDCManager:            oidcManager,
		Storage:                sqlStorage,
		BucketStorage:          bucketStorage,
		EmailService:           emailService,
		DevAuth:                false, // test production behavior
		RefreshTokenExpiration: 90 * 24 * time.Hour,
	})

	return service, sqlStorage, userManager, authTokenConfig
}

// createLegacyPasswordUser writes a pre-#2571 account shape directly: a
// password hash and no verified-email stamp.
//
// Registration no longer produces this shape outside dev mode — #2864 requires
// proof of address ownership — but the accounts that predate the code flow have
// exactly it, and they must keep signing in until they are retired. Tests that
// exercise the password sign-in path build their fixture here rather than
// through EmailRegister, so they state the account shape they depend on instead
// of routing through a registration branch production no longer takes.
//
// Leaves with the password path.
func createLegacyPasswordUser(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, addr, name, password string) *models.User {
	t.Helper()

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password for %s: %v", addr, err)
	}

	now := time.Now().Unix()
	user := &models.User{
		Id:           uuid.New().String(),
		Email:        auth.NormalizeEmail(addr),
		Name:         name,
		CreatedAt:    now,
		UpdatedAt:    now,
		Role:         models.Role_ROLE_USER,
		AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
		PasswordHash: hash,
	}
	if _, err := sqlStorage.Insert(ctx, user); err != nil {
		t.Fatalf("failed to insert legacy password user %s: %v", addr, err)
	}
	return user
}

// createTestCommunity creates a test community for invitation testing.
func createTestCommunity(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, creatorID string) *models.Community {
	community := &models.Community{
		Id:               uuid.New().String(),
		Name:             "Test Community",
		Description:      "A test community",
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: time.Now().Unix(),
		UpdatedAtUnixSec: time.Now().Unix(),
	}

	_, err := sqlStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	return community
}

// createTestInvitation returns the active community-invite share link for
// (inviterID, communityID), creating one if it doesn't already exist.
// Matches GetOrCreateShareLink's community-invite semantics — one
// community-invite link per (inviter, community) is enforced by a partial
// unique index on share_link.
func createTestInvitation(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID string) *models.ShareLink {
	existing, err := sqlStorage.QueryByFields(ctx, map[string]any{
		"inviter_id":          inviterID,
		"community_id":        communityID,
		"community_invite_id": communityID,
	}, &models.ShareLink{})
	if err != nil {
		t.Fatalf("Failed to look up existing test invitation: %v", err)
	}
	if len(existing) > 0 {
		return existing[0].(*models.ShareLink)
	}

	invitation := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		IsRevoked:        false,
		CreatedAtUnixSec: time.Now().Unix(),
		Target: &models.ShareLink_CommunityInviteId{
			CommunityInviteId: communityID,
		},
	}

	if _, err := sqlStorage.Insert(ctx, invitation); err != nil {
		t.Fatalf("Failed to create test invitation: %v", err)
	}

	return invitation
}

// generateTestShortCode generates a random 8-char short code for test invitations.
func generateTestShortCode(t *testing.T) string {
	t.Helper()
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			t.Fatalf("Failed to generate test short code: %v", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// createTestUser creates a test user directly in the database.
func createTestUser(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, email, name string) *models.User {
	now := time.Now().Unix()
	user := &models.User{
		Id:         uuid.New().String(),
		Email:      email,
		Name:       name,
		CreatedAt:  now,
		UpdatedAt:  now,
		Role:       models.Role_ROLE_USER,
		AuthMethod: models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
	}

	_, err := sqlStorage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return user
}

// createServiceWithMockOIDC creates a login service with a mock OIDC provider.
func createServiceWithMockOIDC(t *testing.T) (*Service, *auth.TokenConfig, *auth.MockOIDCSetup) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Create local bucket storage for testing
	tmpDir := t.TempDir()
	bucketStorage, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	// Create test token config
	authTokenConfig := &auth.TokenConfig{
		Secret:     []byte("test-secret-key"),
		Expiration: time.Hour,
	}

	// Create user manager
	userManager := auth.NewUserManager(sqlStorage)

	// Create OIDC manager with mock Google provider
	clientID := "test-client-id.apps.googleusercontent.com"
	oidcSetup := auth.SetupMockGoogleOIDC(t, clientID)

	// Create mock email service
	emailService := &email.MockEmailService{}

	service := New(Config{
		UserManager:            userManager,
		AuthTokenConfig:        authTokenConfig,
		OIDCManager:            oidcSetup.Manager,
		Storage:                sqlStorage,
		BucketStorage:          bucketStorage,
		EmailService:           emailService,
		DevAuth:                false, // test production behavior
		RefreshTokenExpiration: 90 * 24 * time.Hour,
	})

	return service, authTokenConfig, oidcSetup
}

func TestNew(t *testing.T) {
	service, sqlStorage, userManager, authTokenConfig := setupTestService(t)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.userManager != userManager {
		t.Error("Expected userManager to be set correctly")
	}

	if service.authTokenConfig != authTokenConfig {
		t.Error("Expected authTokenConfig to be set correctly")
	}

	if service.storage != sqlStorage {
		t.Error("Expected storage to be set correctly")
	}
}
