package login

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/storage"
)

// mockPhoneTokenVerifier implements auth.PhoneTokenVerifier for testing.
type mockPhoneTokenVerifier struct {
	phoneNumber string
	err         error
}

func (m *mockPhoneTokenVerifier) VerifyPhoneToken(_ context.Context, _ string) (string, error) {
	return m.phoneNumber, m.err
}

// setupPhoneTestService creates a login service with a mock phone auth verifier.
func setupPhoneTestService(t *testing.T, mockPhone *mockPhoneTokenVerifier) (*Service, *storage.ProtoSQLStorage, *auth.UserManager) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	tmpDir := t.TempDir()
	bucketStorage, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	authTokenConfig := &auth.TokenConfig{
		Secret:     []byte("test-secret-key"),
		Expiration: time.Hour,
	}

	userManager := auth.NewUserManager(sqlStorage)
	oidcManager := auth.NewOIDCProviderManager()
	emailService := &email.MockEmailService{}

	service := New(Config{
		UserManager:            userManager,
		AuthTokenConfig:        authTokenConfig,
		OIDCManager:            oidcManager,
		PhoneAuth:              mockPhone,
		Storage:                sqlStorage,
		BucketStorage:          bucketStorage,
		EmailService:           emailService,
		DevAuth:                false,
		RefreshTokenExpiration: 90 * 24 * time.Hour,
	})

	return service, sqlStorage, userManager
}

// setupPhoneTestServiceWithSMS is setupPhoneTestService plus a real notification
// service wired to a mock SMS sender, so tests can assert the opt-in welcome
// text is dispatched end-to-end (PhoneRegister -> notification service -> SMS).
func setupPhoneTestServiceWithSMS(t *testing.T, mockPhone *mockPhoneTokenVerifier) (*Service, *storage.ProtoSQLStorage, *auth.UserManager, *sms.MockSMSSender) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	tmpDir := t.TempDir()
	bucketStorage, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	authTokenConfig := &auth.TokenConfig{Secret: []byte("test-secret-key"), Expiration: time.Hour}
	userManager := auth.NewUserManager(sqlStorage)
	oidcManager := auth.NewOIDCProviderManager()
	emailService := &email.MockEmailService{}

	smsSender := &sms.MockSMSSender{}
	notifService := notifications.NewService(nil, sqlStorage, notifications.WithSMS(notifications.SMSChannel{
		Enabled:    true,
		Sender:     smsSender,
		AppBaseURL: "https://test.example.com",
	}))

	service := New(Config{
		UserManager:            userManager,
		AuthTokenConfig:        authTokenConfig,
		OIDCManager:            oidcManager,
		PhoneAuth:              mockPhone,
		Storage:                sqlStorage,
		BucketStorage:          bucketStorage,
		EmailService:           emailService,
		NotificationService:    notifService,
		Bus:                    cebus.NewMockBus(),
		DevAuth:                false,
		RefreshTokenExpiration: 90 * 24 * time.Hour,
	})

	return service, sqlStorage, userManager, smsSender
}

func TestService_PhoneRegister(t *testing.T) {
	t.Run("successful registration with invitation", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: "+15551234567"}
		service, sqlStorage, userManager := setupPhoneTestService(t, mock)
		ctx := context.Background()

		// Create inviter and community.
		inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create inviter: %v", err)
		}
		community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-firebase-token",
			Name:            "Phone User",
			ShortCode:       invitation.ShortCode,
		})

		resp, err := service.PhoneRegister(ctx, req)
		if err != nil {
			t.Fatalf("PhoneRegister failed: %v", err)
		}

		if resp.Msg.Tokens == nil || resp.Msg.Tokens.AccessToken == "" {
			t.Error("Expected access token to be generated")
		}
		if resp.Msg.Tokens.RefreshToken == "" {
			t.Error("Expected refresh token to be generated")
		}
		if resp.Msg.User == nil {
			t.Fatal("Expected user to be returned")
		}
		if resp.Msg.User.Name != "Phone User" {
			t.Errorf("Expected name 'Phone User', got %q", resp.Msg.User.Name)
		}

		// Verify user was stored with correct auth method and phone number.
		storedUser := &models.User{}
		if err := sqlStorage.GetByID(ctx, resp.Msg.User.Id, storedUser); err != nil {
			t.Fatalf("Failed to get stored user: %v", err)
		}
		if storedUser.AuthMethod != models.AuthMethod_AUTH_METHOD_PHONE {
			t.Errorf("Expected AUTH_METHOD_PHONE, got %v", storedUser.AuthMethod)
		}
		if storedUser.PhoneNumber == nil || *storedUser.PhoneNumber != "+15551234567" {
			t.Errorf("Expected phone number +15551234567, got %v", storedUser.PhoneNumber)
		}
		if storedUser.PasswordHash != "" {
			t.Error("Phone users should not have a password hash")
		}
	})

	t.Run("invalid firebase token", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{err: fmt.Errorf("token expired")}
		service, sqlStorage, userManager := setupPhoneTestService(t, mock)
		ctx := context.Background()

		inviter, _ := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
		community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "bad-token",
			Name:            "Phone User",
			ShortCode:       invitation.ShortCode,
		})

		_, err := service.PhoneRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error for invalid token")
		}
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("duplicate phone number", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: "+15559999999"}
		service, sqlStorage, userManager := setupPhoneTestService(t, mock)
		ctx := context.Background()

		inviter, _ := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
		community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		// Create existing phone user directly.
		phone := "+15559999999"
		now := time.Now().Unix()
		_, err := sqlStorage.Insert(ctx, &models.User{
			Id:          uuid.New().String(),
			Name:        "Existing Phone User",
			PhoneNumber: &phone,
			CreatedAt:   now,
			UpdatedAt:   now,
			Role:        models.Role_ROLE_USER,
			AuthMethod:  models.AuthMethod_AUTH_METHOD_PHONE,
		})
		if err != nil {
			t.Fatalf("Failed to create existing user: %v", err)
		}

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-token",
			Name:            "Duplicate User",
			ShortCode:       invitation.ShortCode,
		})

		_, err = service.PhoneRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error for duplicate phone number")
		}
		if connect.CodeOf(err) != connect.CodeAlreadyExists {
			t.Errorf("Expected AlreadyExists error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("missing name", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: "+15551234567"}
		service, _, _ := setupPhoneTestService(t, mock)
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-token",
			Name:            "",
			ShortCode:       "some-code",
		})

		_, err := service.PhoneRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error for missing name")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("phone auth not configured", func(t *testing.T) {
		// Create service without phone auth.
		service, _, _ := setupPhoneTestService(t, nil)
		// Override: set phoneAuth to nil
		service.phoneAuth = nil
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-token",
			Name:            "Test",
			ShortCode:       "code",
		})

		_, err := service.PhoneRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error when phone auth is not configured")
		}
		if connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Errorf("Expected Unimplemented error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("typed nil phone auth does not panic", func(t *testing.T) {
		// Reproduces the prod nil-interface bug: a typed nil (*FirebaseAuth)(nil)
		// stored in an interface is non-nil, so a plain == nil check passes
		// and the method call panics with a nil pointer dereference.
		service, _, _ := setupPhoneTestService(t, nil)
		var typedNil *auth.FirebaseAuth
		service.phoneAuth = typedNil // interface wraps (*FirebaseAuth)(nil) — non-nil!
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-token",
			Name:            "Test",
			ShortCode:       "code",
		})

		_, err := service.PhoneRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error when phone auth is typed nil")
		}
		if connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Errorf("Expected Unimplemented error, got %v", connect.CodeOf(err))
		}
	})
}

// TestService_PhoneRegister_SendsWelcomeSMS verifies the opt-in welcome text is
// dispatched end-to-end on first registration: PhoneRegister -> notification
// service -> the (mock) SMS sender. This is the live send a carrier reviewer
// relies on to confirm the opt-in flow works during RCS/A2P registration.
func TestService_PhoneRegister_SendsWelcomeSMS(t *testing.T) {
	mock := &mockPhoneTokenVerifier{phoneNumber: "+15557654321"}
	service, sqlStorage, userManager, smsSender := setupPhoneTestServiceWithSMS(t, mock)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	req := connect.NewRequest(&api.PhoneRegisterRequest{
		FirebaseIdToken: "valid-firebase-token",
		Name:            "Welcome User",
		ShortCode:       invitation.ShortCode,
	})
	if _, err := service.PhoneRegister(ctx, req); err != nil {
		t.Fatalf("PhoneRegister failed: %v", err)
	}

	msgs := smsSender.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 welcome SMS on first opt-in, got %d", len(msgs))
	}
	if msgs[0].To != "+15557654321" {
		t.Errorf("welcome To = %q, want +15557654321", msgs[0].To)
	}
	if !strings.Contains(msgs[0].Body, "Welcome to Ripls") {
		t.Errorf("welcome body = %q", msgs[0].Body)
	}
	if !strings.Contains(msgs[0].Body, "STOP to opt out") {
		t.Errorf("welcome missing opt-out disclosure: %q", msgs[0].Body)
	}
}

// TestService_PhoneLogin_NoWelcomeSMS verifies a returning user's login does not
// re-send the welcome — it is a first-opt-in-only message.
func TestService_PhoneLogin_NoWelcomeSMS(t *testing.T) {
	mock := &mockPhoneTokenVerifier{phoneNumber: "+15557654321"}
	service, sqlStorage, _, smsSender := setupPhoneTestServiceWithSMS(t, mock)
	ctx := context.Background()

	phone := "+15557654321"
	now := time.Now().Unix()
	existing := &models.User{
		Id:          uuid.New().String(),
		Name:        "Returning User",
		PhoneNumber: &phone,
		CreatedAt:   now,
		UpdatedAt:   now,
		Role:        models.Role_ROLE_USER,
		AuthMethod:  models.AuthMethod_AUTH_METHOD_PHONE,
	}
	if _, err := sqlStorage.Insert(ctx, existing); err != nil {
		t.Fatalf("Failed to insert existing user: %v", err)
	}

	req := connect.NewRequest(&api.PhoneLoginRequest{FirebaseIdToken: "valid-token"})
	if _, err := service.PhoneLogin(ctx, req); err != nil {
		t.Fatalf("PhoneLogin failed: %v", err)
	}

	if got := len(smsSender.Messages()); got != 0 {
		t.Errorf("login of a returning user must not send a welcome; got %d SMS", got)
	}
}

func TestService_PhoneLogin(t *testing.T) {
	t.Run("successful login", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: "+15551234567"}
		service, sqlStorage, _ := setupPhoneTestService(t, mock)
		ctx := context.Background()

		// Create existing phone user.
		phone := "+15551234567"
		now := time.Now().Unix()
		existingUser := &models.User{
			Id:          uuid.New().String(),
			Name:        "Existing User",
			PhoneNumber: &phone,
			CreatedAt:   now,
			UpdatedAt:   now,
			Role:        models.Role_ROLE_USER,
			AuthMethod:  models.AuthMethod_AUTH_METHOD_PHONE,
		}
		if _, err := sqlStorage.Insert(ctx, existingUser); err != nil {
			t.Fatalf("Failed to create existing user: %v", err)
		}

		req := connect.NewRequest(&api.PhoneLoginRequest{
			FirebaseIdToken: "valid-token",
		})

		resp, err := service.PhoneLogin(ctx, req)
		if err != nil {
			t.Fatalf("PhoneLogin failed: %v", err)
		}

		if resp.Msg.Tokens == nil || resp.Msg.Tokens.AccessToken == "" {
			t.Error("Expected access token")
		}
		if resp.Msg.User.Id != existingUser.Id {
			t.Errorf("Expected user ID %q, got %q", existingUser.Id, resp.Msg.User.Id)
		}
	})

	t.Run("unregistered phone", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: "+15550000000"}
		service, _, _ := setupPhoneTestService(t, mock)
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneLoginRequest{
			FirebaseIdToken: "valid-token",
		})

		_, err := service.PhoneLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unregistered phone")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("additive: email-primary account with an attached phone can phone-login", func(t *testing.T) {
		// Auth methods are additive (#2596): an email/password user who attached
		// a phone (auth_method stays EMAIL_PASSWORD, phone_number set) can sign in
		// with phone OTP. PhoneLogin no longer gates on auth_method — finding the
		// account by its verified phone is proof of the phone credential. This is
		// the login-layer half of the AddPhoneNumber additive contract.
		phone := "+15551112222"
		mock := &mockPhoneTokenVerifier{phoneNumber: phone}
		service, sqlStorage, _ := setupPhoneTestService(t, mock)
		ctx := context.Background()

		now := time.Now().Unix()
		userID := uuid.New().String()
		_, err := sqlStorage.Insert(ctx, &models.User{
			Id:           userID,
			Email:        "email-user@example.com",
			Name:         "Email User",
			PhoneNumber:  &phone,
			CreatedAt:    now,
			UpdatedAt:    now,
			Role:         models.Role_ROLE_USER,
			AuthMethod:   models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD,
			PasswordHash: "some-hash",
		})
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		resp, err := service.PhoneLogin(ctx, connect.NewRequest(&api.PhoneLoginRequest{
			FirebaseIdToken: "valid-token",
		}))
		if err != nil {
			t.Fatalf("expected phone login to succeed for email-primary account with attached phone, got: %v", err)
		}
		if resp.Msg.User.Id != userID {
			t.Errorf("logged in user id = %q, want %q", resp.Msg.User.Id, userID)
		}
		if resp.Msg.Tokens.GetAccessToken() == "" || resp.Msg.Tokens.GetRefreshToken() == "" {
			t.Error("expected access + refresh tokens on successful phone login")
		}
	})

	t.Run("invalid firebase token", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{err: fmt.Errorf("invalid token")}
		service, _, _ := setupPhoneTestService(t, mock)
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneLoginRequest{
			FirebaseIdToken: "bad-token",
		})

		_, err := service.PhoneLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error for invalid token")
		}
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connect.CodeOf(err))
		}
	})

	t.Run("typed nil phone auth does not panic", func(t *testing.T) {
		service, _, _ := setupPhoneTestService(t, nil)
		var typedNil *auth.FirebaseAuth
		service.phoneAuth = typedNil
		ctx := context.Background()

		req := connect.NewRequest(&api.PhoneLoginRequest{
			FirebaseIdToken: "valid-token",
		})

		_, err := service.PhoneLogin(ctx, req)
		if err == nil {
			t.Fatal("Expected error when phone auth is typed nil")
		}
		if connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Errorf("Expected Unimplemented error, got %v", connect.CodeOf(err))
		}
	})
}

// insertPhoneProvisional inserts a phone-keyed provisional user and returns its ID.
func insertPhoneProvisional(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, creatorID, name, phone string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		Id:              uuid.New().String(),
		Name:            name,
		CommunityId:     communityID,
		CreatedByUserId: creatorID,
		Contact:         &models.ProvisionalUser_PhoneNumber{PhoneNumber: phone},
	}
	if _, err := sqlStorage.Insert(ctx, prov); err != nil {
		t.Fatalf("insert phone provisional: %v", err)
	}
	return prov.Id
}

// insertTestExperience inserts a minimal experience and returns its ID.
func insertTestExperience(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage) string {
	t.Helper()
	exp := &models.Experience{
		Id:    uuid.New().String(),
		Name:  "Test Experience",
		State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	if _, err := sqlStorage.Insert(ctx, exp); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	return exp.Id
}

// insertProvisionalRSVP inserts an RSVP owned by a provisional user and returns its ID.
func insertProvisionalRSVP(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, experienceID, communityID, provisionalUserID string) string {
	t.Helper()
	provID := provisionalUserID
	rsvp := &models.ExperienceRSVP{
		Id:                uuid.New().String(),
		ExperienceId:      experienceID,
		CommunityId:       communityID,
		ProvisionalUserId: &provID,
		Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
	}
	if _, err := sqlStorage.Insert(ctx, rsvp); err != nil {
		t.Fatalf("insert provisional RSVP: %v", err)
	}
	return rsvp.Id
}

// TestService_PhoneRegister_PromotesProvisionalUsers covers ID-2: verifying a
// phone promotes every provisional placeholder seeded for that number into the
// new real account — across communities, deduped, and idempotent.
func TestService_PhoneRegister_PromotesProvisionalUsers(t *testing.T) {
	const phone = "+15551234567"

	t.Run("promotes provisional placeholders across communities", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: phone}
		service, sqlStorage, userManager := setupPhoneTestService(t, mock)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}

		// The user registers through commA's link; commB is a separate community
		// they were also invited to as a provisional placeholder.
		commA := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		commB := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, commA.Id, inviter.Id)

		provA := insertPhoneProvisional(t, ctx, sqlStorage, commA.Id, inviter.Id, "Pat", phone)
		provB := insertPhoneProvisional(t, ctx, sqlStorage, commB.Id, inviter.Id, "Pat", phone)

		expA := insertTestExperience(t, ctx, sqlStorage)
		expB := insertTestExperience(t, ctx, sqlStorage)
		rsvpA := insertProvisionalRSVP(t, ctx, sqlStorage, expA, commA.Id, provA)
		rsvpB := insertProvisionalRSVP(t, ctx, sqlStorage, expB, commB.Id, provB)

		resp, err := service.PhoneRegister(ctx, connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-firebase-token",
			Name:            "Pat",
			ShortCode:       invitation.ShortCode,
		}))
		if err != nil {
			t.Fatalf("PhoneRegister: %v", err)
		}
		userID := resp.Msg.User.Id

		// Member of BOTH communities: commA via the link, commB via promotion.
		for _, c := range []string{commA.Id, commB.Id} {
			isMember, err := auth.IsMemberOfCommunity(ctx, sqlStorage, c, userID)
			if err != nil {
				t.Fatalf("IsMemberOfCommunity(%s): %v", c, err)
			}
			if !isMember {
				t.Errorf("user should be a member of community %s after promotion", c)
			}
		}

		// Both provisional placeholders claimed by the new account.
		for _, id := range []string{provA, provB} {
			p := &models.ProvisionalUser{}
			if err := sqlStorage.GetByID(ctx, id, p); err != nil {
				t.Fatalf("get provisional %s: %v", id, err)
			}
			if p.ClaimedByUserId == nil || *p.ClaimedByUserId != userID {
				t.Errorf("provisional %s claimed_by = %v, want %s", id, p.ClaimedByUserId, userID)
			}
		}

		// Both RSVPs migrated to the real user.
		for _, id := range []string{rsvpA, rsvpB} {
			r := &models.ExperienceRSVP{}
			if err := sqlStorage.GetByID(ctx, id, r); err != nil {
				t.Fatalf("get rsvp %s: %v", id, err)
			}
			if r.UserId != userID {
				t.Errorf("rsvp %s user_id = %q, want %q", id, r.UserId, userID)
			}
			if r.ProvisionalUserId != nil {
				t.Errorf("rsvp %s still references provisional user", id)
			}
		}
	})

	t.Run("idempotent when the invite link is tied to the provisional user", func(t *testing.T) {
		mock := &mockPhoneTokenVerifier{phoneNumber: phone}
		service, sqlStorage, userManager := setupPhoneTestService(t, mock)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter2@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}
		comm := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		prov := insertPhoneProvisional(t, ctx, sqlStorage, comm.Id, inviter.Id, "Pat", phone)

		// Link tied to the provisional user: completeRegistration merges + joins,
		// then promotion finds the same phone match — it must not double-join.
		invitation := createTestInvitation(t, ctx, sqlStorage, comm.Id, inviter.Id)
		invitation.ProvisionalUserId = &prov
		if err := sqlStorage.Update(ctx, invitation); err != nil {
			t.Fatalf("update invitation: %v", err)
		}

		resp, err := service.PhoneRegister(ctx, connect.NewRequest(&api.PhoneRegisterRequest{
			FirebaseIdToken: "valid-firebase-token",
			Name:            "Pat",
			ShortCode:       invitation.ShortCode,
		}))
		if err != nil {
			t.Fatalf("PhoneRegister: %v", err)
		}
		userID := resp.Msg.User.Id

		// Exactly one membership row — the link join and the promotion join must
		// not produce a duplicate.
		members, err := sqlStorage.QueryByFields(ctx, map[string]any{
			"community_id": comm.Id,
			"user_id":      userID,
		}, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("query memberships: %v", err)
		}
		if len(members) != 1 {
			t.Errorf("got %d membership rows, want exactly 1 (idempotent join)", len(members))
		}

		p := &models.ProvisionalUser{}
		if err := sqlStorage.GetByID(ctx, prov, p); err != nil {
			t.Fatalf("get provisional: %v", err)
		}
		if p.ClaimedByUserId == nil || *p.ClaimedByUserId != userID {
			t.Errorf("provisional claimed_by = %v, want %s", p.ClaimedByUserId, userID)
		}
	})
}
