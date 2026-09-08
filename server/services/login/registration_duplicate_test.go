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

func TestService_EmailRegister_DuplicateRejection(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-dup@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)

	registerEmail := func(t *testing.T, email string) (*connect.Response[api.EmailRegisterResponse], error) {
		t.Helper()
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)
		return service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:           email,
			Name:            "Test",
			EmailProofToken: proveEmail(t, ctx, service, email),
			ShortCode:       invitation.ShortCode,
		}))
	}

	assertAlreadyExists := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected duplicate registration to fail, got nil error")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected *connect.Error, got %T: %v", err, err)
		}
		if connectErr.Code() != connect.CodeAlreadyExists {
			t.Errorf("expected CodeAlreadyExists, got %v: %s", connectErr.Code(), connectErr.Message())
		}
	}

	countUsersByEmail := func(t *testing.T, email string) int {
		t.Helper()
		results, err := sqlStorage.QueryByField(ctx, "email", auth.NormalizeEmail(email), &models.User{})
		if err != nil {
			t.Fatalf("QueryByField failed: %v", err)
		}
		return len(results)
	}

	t.Run("sequential email register with same email", func(t *testing.T) {
		email := "dup1@example.com"
		if _, err := registerEmail(t, email); err != nil {
			t.Fatalf("first registration failed: %v", err)
		}
		_, err := registerEmail(t, email)
		assertAlreadyExists(t, err)
		if got := countUsersByEmail(t, email); got != 1 {
			t.Errorf("expected exactly 1 user row, got %d", got)
		}
	})

	t.Run("case and whitespace variants collide", func(t *testing.T) {
		if _, err := registerEmail(t, "Foo@Example.Com"); err != nil {
			t.Fatalf("first registration failed: %v", err)
		}
		_, err := registerEmail(t, "  foo@example.com  ")
		assertAlreadyExists(t, err)
		_, err = registerEmail(t, "FOO@EXAMPLE.COM")
		assertAlreadyExists(t, err)
		if got := countUsersByEmail(t, "foo@example.com"); got != 1 {
			t.Errorf("expected exactly 1 user row across case variants, got %d", got)
		}
	})

	t.Run("rejects EmailRegister when an OIDC row already exists", func(t *testing.T) {
		email := "cross@example.com"
		oidcUser := &models.User{
			Id:         uuid.New().String(),
			Email:      auth.NormalizeEmail(email),
			Name:       "Cross",
			AuthMethod: models.AuthMethod_AUTH_METHOD_GOOGLE,
			Role:       models.Role_ROLE_USER,
			CreatedAt:  time.Now().Unix(),
			UpdatedAt:  time.Now().Unix(),
		}
		if _, err := sqlStorage.Insert(ctx, oidcUser); err != nil {
			t.Fatalf("seed OIDC user failed: %v", err)
		}

		_, err := registerEmail(t, email)
		assertAlreadyExists(t, err)

		connectErr := err.(*connect.Error)
		if !strings.Contains(connectErr.Message(), "Google") {
			t.Errorf("expected error message to name existing auth method (Google), got: %s", connectErr.Message())
		}
		if got := countUsersByEmail(t, email); got != 1 {
			t.Errorf("expected exactly 1 user row, got %d", got)
		}
	})

	t.Run("re-register after soft-delete succeeds", func(t *testing.T) {
		email := "rebirth@example.com"
		first, err := registerEmail(t, email)
		if err != nil {
			t.Fatalf("first registration failed: %v", err)
		}
		if err := userManager.DeleteUser(ctx, first.Msg.User.Id); err != nil {
			t.Fatalf("DeleteUser failed: %v", err)
		}
		if _, err := registerEmail(t, email); err != nil {
			t.Errorf("re-registration after soft-delete failed: %v", err)
		}
	})
}
