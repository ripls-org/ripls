package login

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_Register_BootstrapFirstUser(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	t.Run("first user can register with any invitation token", func(t *testing.T) {
		// Verify database is empty
		hasUsers, err := sqlStorage.HasAnyUsers(ctx)
		if err != nil {
			t.Fatalf("Failed to check users: %v", err)
		}
		if hasUsers {
			t.Fatal("Expected empty database for bootstrap test")
		}

		// Register first user with an invalid/non-existent token
		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "bootstrap@example.com",
			Name:            "Bootstrap User",
			EmailProofToken: proveEmail(t, ctx, service, "bootstrap@example.com"),
			ShortCode:       "any-random-invalid-token",
		})

		resp, err := service.EmailRegister(ctx, req)
		if err != nil {
			t.Fatalf("Bootstrap registration should succeed with any token: %v", err)
		}

		// Verify user was created
		if resp.Msg.User.Id == "" {
			t.Error("Expected user ID to be generated")
		}

		// Verify tokens were generated
		if resp.Msg.Tokens == nil || resp.Msg.Tokens.AccessToken == "" {
			t.Error("Expected access token to be generated")
		}

		// Verify user exists in database
		user := &models.User{}
		err = sqlStorage.GetByID(ctx, resp.Msg.User.Id, user)
		if err != nil {
			t.Fatalf("Failed to retrieve user: %v", err)
		}

		if user.Email != "bootstrap@example.com" {
			t.Errorf("Expected email bootstrap@example.com, got %s", user.Email)
		}

		// Verify user is not part of any community (bootstrap user)
		memberships, err := sqlStorage.QueryByField(ctx, "user_id", user.Id, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("Failed to query memberships: %v", err)
		}
		if len(memberships) != 0 {
			t.Errorf("Expected bootstrap user to have no community memberships, got %d", len(memberships))
		}
	})

	t.Run("second user requires valid invitation token", func(t *testing.T) {
		// Create a new service with a single user already registered
		service2, _, _, _ := setupTestService(t)
		ctx2 := context.Background()

		// Register first user
		firstUserReq := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "first@example.com",
			Name:            "First User",
			EmailProofToken: proveEmail(t, ctx2, service2, "first@example.com"),
			ShortCode:       "any-token-works",
		})
		_, err := service2.EmailRegister(ctx2, firstUserReq)
		if err != nil {
			t.Fatalf("Failed to register first user: %v", err)
		}

		// Try to register second user with invalid token - should fail
		secondUserReq := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "second@example.com",
			Name:            "Second User",
			EmailProofToken: proveEmail(t, ctx2, service2, "second@example.com"),
			ShortCode:       "invalid-token",
		})

		_, err = service2.EmailRegister(ctx2, secondUserReq)
		if err == nil {
			t.Fatal("Expected error for invalid invitation token after first user")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}

		if !strings.Contains(connectErr.Message(), "invalid invitation code") {
			t.Errorf("Expected error about invalid code, got: %s", connectErr.Message())
		}
	})

	t.Run("bootstrap user cannot register without token", func(t *testing.T) {
		// Even the first user needs SOME token (though it can be invalid)
		service3, _, _, _ := setupTestService(t)
		ctx3 := context.Background()

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "notoken@example.com",
			Name:            "No Token User",
			EmailProofToken: proveEmail(t, ctx3, service3, "notoken@example.com"),
			ShortCode:       "", // Empty short code
		})

		_, err := service3.EmailRegister(ctx3, req)
		if err == nil {
			t.Fatal("Expected error for missing invitation token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})
}

func TestService_Register_CommunitySizeLimit(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	// Create an inviter user and community
	inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	testCommunity := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, testCommunity.Id, inviter.Id)

	// Add inviter as first member of the community
	inviterMembership := &models.CommunityUser{
		Id:               uuid.New().String(),
		CommunityId:      testCommunity.Id,
		UserId:           inviter.Id,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := sqlStorage.Insert(ctx, inviterMembership); err != nil {
		t.Fatalf("Failed to add inviter to community: %v", err)
	}

	// Add 31 more members to reach capacity (inviter is member #1)
	// MaxCommunityMembers is 32, so we need 31 more members
	for i := 0; i < 31; i++ {
		memberEmail := "member" + string(rune('a'+i/26)) + string(rune('a'+i%26)) + "@example.com"
		member, err := userManager.CreateUser(ctx, memberEmail, "Member "+string(rune('A'+i)), models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create member %d: %v", i, err)
		}
		membership := &models.CommunityUser{
			Id:               uuid.New().String(),
			CommunityId:      testCommunity.Id,
			UserId:           member.Id,
			InviterId:        inviter.Id,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		if _, err := sqlStorage.Insert(ctx, membership); err != nil {
			t.Fatalf("Failed to add member %d to community: %v", i, err)
		}
	}

	t.Run("registration fails when community is at capacity", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "newuser@example.com",
			Name:            "New User",
			EmailProofToken: proveEmail(t, ctx, service, "newuser@example.com"),
			ShortCode:       invitation.ShortCode,
		})

		_, err := service.EmailRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error when registering to full community")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeResourceExhausted {
			t.Errorf("Expected ResourceExhausted error, got %v", connectErr.Code())
		}

		if !strings.Contains(connectErr.Message(), "maximum capacity") {
			t.Errorf("Expected error message about capacity, got: %s", connectErr.Message())
		}
	})
}
