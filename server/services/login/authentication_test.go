package login

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_Login(t *testing.T) {
	service, sqlStorage, _, authTokenConfig := setupTestService(t)
	ctx := context.Background()

	// A legacy password account, written directly: registration will not create
	// one any more (#2864), and this test is about signing one in.
	testPassword := "testpassword123"
	testUser := createLegacyPasswordUser(t, ctx, sqlStorage, "login@example.com", "Login User", testPassword)

	// Add a media_id to test avatar functionality.
	testUser.MediaIds = []string{"test-media-id-123"}
	if err := sqlStorage.Update(ctx, testUser); err != nil {
		t.Fatalf("Failed to update test user with media_ids: %v", err)
	}

	t.Run("successful login", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailLoginRequest{
			Email:    "login@example.com",
			Password: testPassword,
		})

		resp, err := service.EmailLogin(ctx, req)
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		loginResp := resp.Msg

		// Verify tokens are generated
		if loginResp.Tokens == nil || loginResp.Tokens.AccessToken == "" {
			t.Error("Expected access token to be generated")
		}
		if loginResp.Tokens.RefreshToken == "" {
			t.Error("Expected refresh token to be generated")
		}

		// Verify user object is populated
		if loginResp.User == nil {
			t.Fatal("Expected user object to be populated")
		}
		if loginResp.User.Id != testUser.Id {
			t.Errorf("Expected user ID %s, got %s", testUser.Id, loginResp.User.Id)
		}
		if loginResp.User.Name != testUser.Name {
			t.Errorf("Expected user name %s, got %s", testUser.Name, loginResp.User.Name)
		}
		if loginResp.User.MediaId != testUser.MediaIds[0] {
			t.Errorf("Expected user media_id %s, got %s", testUser.MediaIds[0], loginResp.User.MediaId)
		}

		// Verify the token is valid
		claims, err := authTokenConfig.ValidateToken(loginResp.Tokens.AccessToken)
		if err != nil {
			t.Fatalf("Generated token is invalid: %v", err)
		}

		if claims["user_id"] != testUser.Id {
			t.Errorf("Token user_id mismatch: expected %s, got %v", testUser.Id, claims["user_id"])
		}

		if claims["email"] != testUser.Email {
			t.Errorf("Token email mismatch: expected %s, got %v", testUser.Email, claims["email"])
		}
	})

	t.Run("missing password", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailLoginRequest{
			Email: "login@example.com",
			// Password is missing
		})

		_, err := service.EmailLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for missing password")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailLoginRequest{
			Email:    "login@example.com",
			Password: "wrongpassword",
		})

		_, err := service.EmailLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for wrong password")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("user not found", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailLoginRequest{
			Email:    "nonexistent@example.com",
			Password: "password123",
		})

		_, err := service.EmailLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		// Could be either Internal or Unauthenticated
		if connectErr.Code() != connect.CodeInternal && connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Internal or Unauthenticated error, got %v", connectErr.Code())
		}
	})
}

func TestService_Login_RefreshTokens(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	// A legacy password account, written directly: this test signs in with a
	// password, which registration no longer issues (#2864).
	createLegacyPasswordUser(t, ctx, sqlStorage, "rt-login@example.com", "RT Login User", "password123")

	t.Run("refresh token from login is usable", func(t *testing.T) {
		loginResp, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:    "rt-login@example.com",
			Password: "password123",
		}))
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		// The refresh token must be accepted by the RefreshToken RPC.
		refreshResp, err := service.RefreshToken(ctx, connect.NewRequest(&api.RefreshTokenRequest{
			RefreshToken: loginResp.Msg.Tokens.RefreshToken,
		}))
		if err != nil {
			t.Fatalf("RefreshToken rejected the token returned by Login: %v", err)
		}
		if refreshResp.Msg.Tokens == nil || refreshResp.Msg.Tokens.AccessToken == "" {
			t.Error("Expected a new access token after refresh")
		}
	})

	t.Run("each login issues a distinct refresh token", func(t *testing.T) {
		login1, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:    "rt-login@example.com",
			Password: "password123",
		}))
		if err != nil {
			t.Fatalf("First login failed: %v", err)
		}
		login2, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:    "rt-login@example.com",
			Password: "password123",
		}))
		if err != nil {
			t.Fatalf("Second login failed: %v", err)
		}
		if login1.Msg.Tokens.RefreshToken == login2.Msg.Tokens.RefreshToken {
			t.Error("Expected distinct refresh tokens for independent login sessions")
		}
	})

	t.Run("failed login does not produce tokens", func(t *testing.T) {
		resp, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:    "rt-login@example.com",
			Password: "wrongpassword",
		}))
		if err == nil {
			t.Fatal("Expected login to fail with wrong password")
		}
		if resp != nil {
			t.Error("Expected nil response on failed login")
		}
	})
}

func TestService_RegisterAndLogin_Integration(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)

	ctx := context.Background()

	// Create test community and invitation
	creator := createTestUser(t, ctx, sqlStorage, "creator@example.com", "Creator")
	community := createTestCommunity(t, ctx, sqlStorage, creator.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, creator.Id)

	// Register a user
	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "integration@example.com",
		Name:            "Integration Test User",
		EmailProofToken: proveEmail(t, ctx, service, "integration@example.com"),
		ShortCode:       invitation.ShortCode,
	})

	registerResp, err := service.EmailRegister(ctx, registerReq)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	registeredUserID := registerResp.Msg.User.Id
	registerToken := registerResp.Msg.Tokens.AccessToken

	// Login with the same user
	loginReq := connect.NewRequest(&api.EmailLoginRequest{
		Email:           "integration@example.com",
		EmailProofToken: proveEmail(t, ctx, service, "integration@example.com"),
	})

	loginResp, err := service.EmailLogin(ctx, loginReq)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	loginUserID := loginResp.Msg.User.Id
	loginToken := loginResp.Msg.Tokens.AccessToken

	// Verify user data is consistent
	if loginUserID != registeredUserID {
		t.Errorf("User ID mismatch: register=%s, login=%s", registeredUserID, loginUserID)
	}

	// Verify both tokens are valid (they will be different due to different generation times)
	authTokenConfig := &auth.TokenConfig{
		Secret:     []byte("test-secret-key"),
		Expiration: time.Hour,
	}

	_, err = authTokenConfig.ValidateToken(registerToken)
	if err != nil {
		t.Errorf("Register token is invalid: %v", err)
	}

	_, err = authTokenConfig.ValidateToken(loginToken)
	if err != nil {
		t.Errorf("Login token is invalid: %v", err)
	}
}

func TestService_MultipleUsers(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)

	ctx := context.Background()

	// Create test community for invitations
	creator := createTestUser(t, ctx, sqlStorage, "creator@example.com", "Creator")
	community := createTestCommunity(t, ctx, sqlStorage, creator.Id)

	users := []struct {
		email    string
		name     string
		password string
	}{
		{"user1@example.com", "User One", "password123"},
		{"user2@example.com", "User Two", "password456"},
		{"user3@example.com", "User Three", "password789"},
	}

	var registeredUserIDs []string

	// Register multiple users
	for _, user := range users {
		// Create invitation for each user
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, creator.Id)

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           user.email,
			Name:            user.name,
			EmailProofToken: proveEmail(t, ctx, service, user.email),
			ShortCode:       invitation.ShortCode,
		})

		resp, err := service.EmailRegister(ctx, req)
		if err != nil {
			t.Fatalf("Failed to register user %s: %v", user.email, err)
		}

		registeredUserIDs = append(registeredUserIDs, resp.Msg.User.Id)
	}

	// Login with each user and verify data
	for i, user := range users {
		loginReq := connect.NewRequest(&api.EmailLoginRequest{
			Email:           user.email,
			EmailProofToken: proveEmail(t, ctx, service, user.email),
		})

		loginResp, err := service.EmailLogin(ctx, loginReq)
		if err != nil {
			t.Fatalf("Failed to login user %s: %v", user.email, err)
		}

		loginUserID := loginResp.Msg.User.Id
		registeredUserID := registeredUserIDs[i]

		if loginUserID != registeredUserID {
			t.Errorf("User %s ID mismatch: register=%s, login=%s",
				user.email, registeredUserID, loginUserID)
		}
	}

	idSet := make(map[string]bool)
	for _, userID := range registeredUserIDs {
		if idSet[userID] {
			t.Errorf("Duplicate user ID found: %s", userID)
		}
		idSet[userID] = true
	}
}

func TestService_OIDCLogin(t *testing.T) {
	t.Run("unspecified provider", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		ctx := context.Background()

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_UNSPECIFIED,
			IdToken:  "test-token",
		})

		_, err := service.OIDCLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unspecified provider")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("missing id_token", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		ctx := context.Background()

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
			IdToken:  "",
		})

		_, err := service.OIDCLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for missing id_token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("provider not registered", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		ctx := context.Background()

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
			IdToken:  "test-token",
		})

		_, err := service.OIDCLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unregistered provider")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		ctx := context.Background()

		// Create a test service with a registered OIDC provider
		service, _, _ := createServiceWithMockOIDC(t)

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
			IdToken:  "invalid-token",
		})

		_, err := service.OIDCLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for invalid token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("OIDC login with new user should fail", func(t *testing.T) {
		ctx := context.Background()

		// Create a test service with a registered OIDC provider
		service, _, oidcSetup := createServiceWithMockOIDC(t)

		// Create a valid test token for a user that doesn't exist
		testToken := oidcSetup.CreateValidGoogleToken(t, "test-user-123", "newuser@example.com", "New User")

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
			IdToken:  testToken,
		})

		_, err := service.OIDCLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for new user - should use OIDCRegister instead")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}

		// Verify error message tells user to register
		if !strings.Contains(connectErr.Message(), "register") {
			t.Errorf("Expected error message to mention registration, got: %s", connectErr.Message())
		}
	})

	t.Run("successful OIDC login with existing user", func(t *testing.T) {
		service, sqlStorage, _, authTokenConfig := setupTestService(t)
		ctx := context.Background()

		// Pre-create a user with Google auth method
		now := time.Now().Unix()
		existingUser := &models.User{
			Id:                  uuid.New().String(),
			Email:               "existing@example.com",
			Name:                "Existing User",
			CreatedAt:           now,
			UpdatedAt:           now,
			Role:                models.Role_ROLE_USER,
			AuthMethod:          models.AuthMethod_AUTH_METHOD_GOOGLE,
			OidcProviderSubject: "existing-user-123",
		}
		_, err := sqlStorage.Insert(ctx, existingUser)
		if err != nil {
			t.Fatalf("Failed to create existing user: %v", err)
		}

		// Set up the service with mock OIDC
		oidcSetup := auth.SetupMockGoogleOIDC(t, "test-client-id.apps.googleusercontent.com")
		service.oidcManager = oidcSetup.Manager

		// Create a valid test token for the existing user
		testToken := oidcSetup.CreateValidGoogleToken(t, "existing-user-123", "existing@example.com", "Updated Name")

		req := connect.NewRequest(&api.OIDCLoginRequest{
			Provider: api.OIDCProvider_OIDC_PROVIDER_GOOGLE,
			IdToken:  testToken,
		})

		resp, err := service.OIDCLogin(ctx, req)
		if err != nil {
			t.Fatalf("OIDC login failed: %v", err)
		}

		loginResp := resp.Msg

		// Verify user data - should be the existing user
		if loginResp.User.Id != existingUser.Id {
			t.Errorf("Expected existing user ID %s, got %s", existingUser.Id, loginResp.User.Id)
		}

		// Verify the token is valid
		claims, err := authTokenConfig.ValidateToken(loginResp.Tokens.AccessToken)
		if err != nil {
			t.Fatalf("Generated token is invalid: %v", err)
		}

		if claims["user_id"] != existingUser.Id {
			t.Errorf("Token user_id mismatch: expected %s, got %v", existingUser.Id, claims["user_id"])
		}
	})
}
