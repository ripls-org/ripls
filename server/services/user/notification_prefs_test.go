package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_GetUserNotificationPreferences_ReturnsEmptyWhenNoRow(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	user, err := userManager.CreateUser(ctx, "prefs-get@example.com", "Prefs Get", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	authCtx := createAuthenticatedContext(user.Id, user.Email, user.Role)

	resp, err := service.GetUserNotificationPreferences(authCtx, connect.NewRequest(&api.GetUserNotificationPreferencesRequest{}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	prefs := resp.Msg.Preferences
	if prefs == nil {
		t.Fatal("expected non-nil preferences")
	}
	if prefs.NotifyLoanReturnReminders != nil {
		t.Errorf("NotifyLoanReturnReminders = %v, want nil (unset)", *prefs.NotifyLoanReturnReminders)
	}
	if prefs.NotifyEventClosePrompts != nil {
		t.Errorf("NotifyEventClosePrompts = %v, want nil (unset)", *prefs.NotifyEventClosePrompts)
	}
}

func TestService_GetUserNotificationPreferences_RequiresAuth(t *testing.T) {
	service, _, _ := setupTestService(t)

	_, err := service.GetUserNotificationPreferences(context.Background(), connect.NewRequest(&api.GetUserNotificationPreferencesRequest{}))
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

func TestService_UpdateUserNotificationPreferences_FirstCallInserts(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	user, err := userManager.CreateUser(ctx, "prefs-update-insert@example.com", "Prefs Insert", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	authCtx := createAuthenticatedContext(user.Id, user.Email, user.Role)

	disabled := false
	resp, err := service.UpdateUserNotificationPreferences(authCtx, connect.NewRequest(&api.UpdateUserNotificationPreferencesRequest{
		Preferences: &api.UserNotificationPreferences{
			NotifyLoanReturnReminders: &disabled,
		},
	}))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := resp.Msg.Preferences
	if got.NotifyLoanReturnReminders == nil || *got.NotifyLoanReturnReminders {
		t.Errorf("NotifyLoanReturnReminders = %v, want false", got.NotifyLoanReturnReminders)
	}
	if got.NotifyEventClosePrompts != nil {
		t.Errorf("NotifyEventClosePrompts should still be unset, got %v", *got.NotifyEventClosePrompts)
	}

	// Read-back: a Get should return the same values.
	getResp, err := service.GetUserNotificationPreferences(authCtx, connect.NewRequest(&api.GetUserNotificationPreferencesRequest{}))
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if getResp.Msg.Preferences.NotifyLoanReturnReminders == nil || *getResp.Msg.Preferences.NotifyLoanReturnReminders {
		t.Errorf("read-back NotifyLoanReturnReminders = %v, want false", getResp.Msg.Preferences.NotifyLoanReturnReminders)
	}
}

func TestService_UpdateUserNotificationPreferences_PartialUpdatePreservesOtherFields(t *testing.T) {
	service, userManager, _ := setupTestService(t)
	ctx := context.Background()

	user, err := userManager.CreateUser(ctx, "prefs-partial@example.com", "Prefs Partial", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	authCtx := createAuthenticatedContext(user.Id, user.Email, user.Role)

	// First update: set both fields explicitly.
	disabled := false
	enabled := true
	if _, err := service.UpdateUserNotificationPreferences(authCtx, connect.NewRequest(&api.UpdateUserNotificationPreferencesRequest{
		Preferences: &api.UserNotificationPreferences{
			NotifyLoanReturnReminders: &disabled,
			NotifyEventClosePrompts:   &enabled,
		},
	})); err != nil {
		t.Fatalf("first Update: %v", err)
	}

	// Second update: only touch NotifyLoanReturnReminders. The close-prompt
	// value should be preserved.
	resp, err := service.UpdateUserNotificationPreferences(authCtx, connect.NewRequest(&api.UpdateUserNotificationPreferencesRequest{
		Preferences: &api.UserNotificationPreferences{
			NotifyLoanReturnReminders: &enabled,
		},
	}))
	if err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if resp.Msg.Preferences.NotifyLoanReturnReminders == nil || !*resp.Msg.Preferences.NotifyLoanReturnReminders {
		t.Errorf("NotifyLoanReturnReminders = %v, want true", resp.Msg.Preferences.NotifyLoanReturnReminders)
	}
	if resp.Msg.Preferences.NotifyEventClosePrompts == nil || !*resp.Msg.Preferences.NotifyEventClosePrompts {
		t.Errorf("NotifyEventClosePrompts should be preserved as true; got %v", resp.Msg.Preferences.NotifyEventClosePrompts)
	}
}

func TestService_UpdateUserNotificationPreferences_RequiresAuth(t *testing.T) {
	service, _, _ := setupTestService(t)

	_, err := service.UpdateUserNotificationPreferences(context.Background(), connect.NewRequest(&api.UpdateUserNotificationPreferencesRequest{}))
	if err == nil {
		t.Fatal("expected unauthenticated error")
	}
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("expected CodeUnauthenticated, got %v", connect.CodeOf(err))
	}
}

func TestLoadUserNotificationPreferences_NilWhenNoRow(t *testing.T) {
	_, userManager, sqlStorage := setupTestService(t)
	ctx := context.Background()

	user, err := userManager.CreateUser(ctx, "load-none@example.com", "Load None", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	prefs, err := LoadUserNotificationPreferences(ctx, sqlStorage, user.Id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if prefs != nil {
		t.Errorf("expected nil prefs for user with no row, got %+v", prefs)
	}
}
