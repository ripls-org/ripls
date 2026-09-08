package login

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_Register(t *testing.T) {
	service, sqlStorage, userManager, authTokenConfig := setupTestService(t)
	ctx := context.Background()

	// Create an inviter user and community for invitation tests
	inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)

	t.Run("successful registration with invitation", func(t *testing.T) {
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "test@example.com",
			Name:            "Test User",
			EmailProofToken: proveEmail(t, ctx, service, "test@example.com"),
			ShortCode:       invitation.ShortCode,
		})

		resp, err := service.EmailRegister(ctx, req)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		loginResp := resp.Msg

		// Verify tokens are generated
		if loginResp.Tokens == nil || loginResp.Tokens.AccessToken == "" {
			t.Error("Expected access token to be generated")
		}
		if loginResp.Tokens.RefreshToken == "" {
			t.Error("Expected refresh token to be generated")
		}

		// Verify user object is populated correctly
		if loginResp.User == nil {
			t.Fatal("Expected user object to be populated")
		}
		if loginResp.User.Id == "" {
			t.Error("Expected user ID to be generated")
		}
		if loginResp.User.Name != req.Msg.Name {
			t.Errorf("Expected user name %s, got %s", req.Msg.Name, loginResp.User.Name)
		}
		// Email/password registration doesn't fetch avatars, so media_id should be empty
		if loginResp.User.MediaId != "" {
			t.Errorf("Expected empty media_id for email/password registration, got %s", loginResp.User.MediaId)
		}

		// Verify the token is valid and contains correct user_id
		claims, err := authTokenConfig.ValidateToken(loginResp.Tokens.AccessToken)
		if err != nil {
			t.Fatalf("Generated token is invalid: %v", err)
		}

		if claims["user_id"] != loginResp.User.Id {
			t.Errorf("Token user_id mismatch: expected %s, got %v", loginResp.User.Id, claims["user_id"])
		}

		// Verify token contains the registered email
		if claims["email"] != req.Msg.Email {
			t.Errorf("Token email mismatch: expected %s, got %v", req.Msg.Email, claims["email"])
		}

		// Verify user was added to community
		memberships, err := sqlStorage.QueryByField(ctx, "user_id", loginResp.User.Id, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("Failed to query memberships: %v", err)
		}
		if len(memberships) != 1 {
			t.Errorf("Expected 1 community membership, got %d", len(memberships))
		}

		// Verify share link still exists (reusable, not deleted)
		invitations, err := sqlStorage.QueryByField(ctx, "short_code", invitation.ShortCode, &models.ShareLink{})
		if err != nil {
			t.Fatalf("Failed to query share links: %v", err)
		}
		if len(invitations) != 1 {
			t.Error("Expected share link to still exist (reusable)")
		}
	})

	t.Run("missing password", func(t *testing.T) {
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:     "test2@example.com",
			Name:      "Test User",
			ShortCode: invitation.ShortCode,
			// Password is missing
		})

		_, err := service.EmailRegister(ctx, req)
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

	t.Run("missing invitation token", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "test3@example.com",
			Name:            "Test User",
			EmailProofToken: proveEmail(t, ctx, service, "test3@example.com"),
			// ShortCode is missing
		})

		_, err := service.EmailRegister(ctx, req)
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

	t.Run("invalid invitation token", func(t *testing.T) {
		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "test4@example.com",
			Name:            "Test User",
			EmailProofToken: proveEmail(t, ctx, service, "test4@example.com"),
			ShortCode:       "invalid-token",
		})

		_, err := service.EmailRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error for invalid invitation token")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("revoked invitation", func(t *testing.T) {
		// Create a revoked community-invite share link
		revokedInvitation := &models.ShareLink{
			Id:               uuid.New().String(),
			CommunityId:      community.Id,
			InviterId:        inviter.Id,
			ShortCode:        generateTestShortCode(t),
			IsRevoked:        true, // Revoked
			CreatedAtUnixSec: time.Now().Unix(),
			Target: &models.ShareLink_CommunityInviteId{
				CommunityInviteId: community.Id,
			},
		}

		_, err := sqlStorage.Insert(ctx, revokedInvitation)
		if err != nil {
			t.Fatalf("Failed to create revoked invitation: %v", err)
		}

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "revoked@example.com",
			Name:            "Test User",
			EmailProofToken: proveEmail(t, ctx, service, "revoked@example.com"),
			ShortCode:       revokedInvitation.ShortCode,
		})

		_, err = service.EmailRegister(ctx, req)
		if err == nil {
			t.Fatal("Expected error for revoked invitation")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
	})

	t.Run("password without a proof token rejected", func(t *testing.T) {
		invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

		req := connect.NewRequest(&api.EmailRegisterRequest{
			Email:     "weak@example.com",
			Name:      "Test User",
			Password:  "short",
			ShortCode: invitation.ShortCode,
		})

		_, err := service.EmailRegister(ctx, req)
		if err == nil {
			t.Fatal("expected a password-only registration to be rejected")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("Expected InvalidArgument error, got %v", connectErr.Code())
		}
		// Assert on the message, not just the code: registration used to reject
		// this for being a weak password, and now rejects it for carrying no
		// proof of address ownership at all (#2864). Both are InvalidArgument,
		// so the code alone would keep passing if the credential requirement
		// silently regressed.
		if got := connectErr.Message(); !strings.Contains(got, "verification code is required") {
			t.Errorf("expected rejection for the missing proof token, got %q", got)
		}
	})
}

// TestService_Register_AllShareLinkVariants confirms registration accepts
// share links of every target variant (community-invite, gear, transfer,
// request, event). The link is used only to join the inviter's community;
// the client routes to the variant-specific landing screen post-sign-in.
// Regression: previously rejected non-community-invite variants with
// "this invitation requires the dedicated accept flow for its target type".
func TestService_Register_AllShareLinkVariants(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "inviter-variants@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("Failed to create inviter: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)

	// Each case builds a share link with a different oneof variant. The
	// wrapper interface is unexported so the table holds a builder rather
	// than the variant value directly.
	cases := []struct {
		name      string
		applyLink func(link *models.ShareLink)
	}{
		{
			name: "community-invite",
			applyLink: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_CommunityInviteId{CommunityInviteId: community.Id}
			},
		},
		{
			name: "gear",
			applyLink: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_GearId{GearId: uuid.New().String()}
			},
		},
		{
			name: "transfer",
			applyLink: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_TransferId{TransferId: uuid.New().String()}
			},
		},
		{
			name: "request",
			applyLink: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_RequestId{RequestId: uuid.New().String()}
			},
		},
		{
			name: "event",
			applyLink: func(link *models.ShareLink) {
				link.Target = &models.ShareLink_ExperienceId{ExperienceId: uuid.New().String()}
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := &models.ShareLink{
				Id:               uuid.New().String(),
				CommunityId:      community.Id,
				InviterId:        inviter.Id,
				ShortCode:        generateTestShortCode(t),
				CreatedAtUnixSec: time.Now().Unix(),
			}
			tc.applyLink(link)
			if _, err := sqlStorage.Insert(ctx, link); err != nil {
				t.Fatalf("insert share link: %v", err)
			}

			req := connect.NewRequest(&api.EmailRegisterRequest{
				Email:           fmt.Sprintf("variant-%d@example.com", i),
				Name:            "Variant User",
				EmailProofToken: proveEmail(t, ctx, service, fmt.Sprintf("variant-%d@example.com", i)),
				ShortCode:       link.ShortCode,
			})

			resp, err := service.EmailRegister(ctx, req)
			if err != nil {
				t.Fatalf("EmailRegister rejected %s share link: %v", tc.name, err)
			}
			if resp.Msg.User == nil || resp.Msg.User.Id == "" {
				t.Fatalf("expected user to be created for %s share link", tc.name)
			}

			// The user should have joined the inviter's community regardless of
			// target variant — that's the whole point of allowing every kind.
			memberships, err := sqlStorage.QueryByFields(ctx, map[string]any{
				"user_id":      resp.Msg.User.Id,
				"community_id": community.Id,
			}, &models.CommunityUser{})
			if err != nil {
				t.Fatalf("query memberships: %v", err)
			}
			if len(memberships) != 1 {
				t.Errorf("expected 1 membership for %s, got %d", tc.name, len(memberships))
			}
		})
	}
}
