package login

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// TestEmailRegister_SeedsPreferredLanguageFromAcceptLanguage verifies
// that the registration path picks up the Accept-Language header
// captured into ctx by the AcceptLanguage middleware and stores the
// normalized tag on the new user row.
func TestEmailRegister_SeedsPreferredLanguageFromAcceptLanguage(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "ll-inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	// Simulate what the middleware does: store the inbound
	// Accept-Language on the request context before the handler
	// runs.
	registerCtx := l10n.WithAcceptLanguage(ctx, "es-MX,en;q=0.9")

	resp, err := service.EmailRegister(registerCtx, connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "ll-spanish@example.com",
		Name:            "Spanish Speaker",
		EmailProofToken: proveEmail(t, ctx, service, "ll-spanish@example.com"),
		ShortCode:       invitation.ShortCode,
	}))
	if err != nil {
		t.Fatalf("EmailRegister: %v", err)
	}

	stored := &models.User{}
	if err := sqlStorage.GetByID(ctx, resp.Msg.User.Id, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.GetPreferredLanguage() != "es" {
		t.Errorf("preferred_language = %q, want %q (normalized from es-MX)",
			stored.GetPreferredLanguage(), "es")
	}
}

// TestEmailRegister_DefaultsPreferredLanguageWithoutHeader verifies
// that a registration request with no Accept-Language signal still
// leaves the user with a usable (English) preference.
func TestEmailRegister_DefaultsPreferredLanguageWithoutHeader(t *testing.T) {
	service, sqlStorage, userManager, _ := setupTestService(t)
	ctx := context.Background()

	inviter, err := userManager.CreateUser(ctx, "ll-default-inviter@example.com", "Inviter", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id)

	resp, err := service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
		Email:           "ll-default@example.com",
		Name:            "Default Lang",
		EmailProofToken: proveEmail(t, ctx, service, "ll-default@example.com"),
		ShortCode:       invitation.ShortCode,
	}))
	if err != nil {
		t.Fatalf("EmailRegister: %v", err)
	}

	stored := &models.User{}
	if err := sqlStorage.GetByID(ctx, resp.Msg.User.Id, stored); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.GetPreferredLanguage() != l10n.DefaultTag.String() {
		t.Errorf("preferred_language = %q, want default %q",
			stored.GetPreferredLanguage(), l10n.DefaultTag.String())
	}
}
