package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// seedCommunityMembership inserts a Community with the given ID (if it
// doesn't already exist) and a CommunityUser row pairing it with the user.
// Required since Phase 4 of #1621 wired RequireMemberOfActiveCommunity
// into the notification-prefs handlers; tests can no longer pass synthetic
// community IDs without matching storage rows.
func seedCommunityMembership(t *testing.T, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	existing := &models.Community{}
	if err := s.GetByID(context.Background(), communityID, existing); err != nil {
		// Not present — create it.
		if _, err := s.Insert(context.Background(), &models.Community{
			Id:          communityID,
			Name:        communityID,
			CreatorId:   userID,
			OwnerUserId: userID,
		}); err != nil {
			t.Fatalf("seed community %s: %v", communityID, err)
		}
	}
	if _, err := s.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}); err != nil {
		t.Fatalf("seed membership %s/%s: %v", communityID, userID, err)
	}
}

func TestGetCommunityNotificationPreferences_MissingRowReturnsAllUnset(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)
	userID := setupTestUser(t, sqlStorage, "alice@test.com", "Alice")
	ctx := createAuthenticatedContext(userID, "alice@test.com", models.Role_ROLE_USER)
	seedCommunityMembership(t, sqlStorage, "community-1", userID)

	resp, err := service.GetCommunityNotificationPreferences(ctx, connect.NewRequest(
		&api.GetCommunityNotificationPreferencesRequest{CommunityId: "community-1"},
	))
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	prefs := resp.Msg.Preferences
	if prefs == nil {
		t.Fatal("expected preferences message in response, got nil")
	}
	// Every toggle should be unset; clients treat unset as "on".
	if prefs.NotifyNewRequests != nil ||
		prefs.NotifyNewExperiences != nil ||
		prefs.NotifyGearShared != nil ||
		prefs.NotifyExperienceCompleted != nil ||
		prefs.NotifyTransferUpdates != nil ||
		prefs.NotifyRequestUpdates != nil ||
		prefs.NotifyExperienceRsvps != nil ||
		prefs.NotifyPlanningUpdates != nil ||
		prefs.NotifyChats != nil {
		t.Error("expected all toggles unset for missing row")
	}
}

func TestUpdateCommunityNotificationPreferences_InsertsAndPreservesUnset(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)
	userID := setupTestUser(t, sqlStorage, "bob@test.com", "Bob")
	ctx := createAuthenticatedContext(userID, "bob@test.com", models.Role_ROLE_USER)
	seedCommunityMembership(t, sqlStorage, "community-1", userID)

	disabled := false
	enabled := true
	updateReq := &api.UpdateCommunityNotificationPreferencesRequest{
		CommunityId: "community-1",
		Preferences: &api.CommunityNotificationPreferences{
			NotifyChats:          &disabled,
			NotifyNewExperiences: &enabled,
			// All other toggles intentionally left unset.
		},
	}
	updateResp, err := service.UpdateCommunityNotificationPreferences(ctx, connect.NewRequest(updateReq))
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updateResp.Msg.Preferences.NotifyChats == nil || *updateResp.Msg.Preferences.NotifyChats {
		t.Error("expected NotifyChats=false in update response")
	}
	if updateResp.Msg.Preferences.NotifyNewExperiences == nil || !*updateResp.Msg.Preferences.NotifyNewExperiences {
		t.Error("expected NotifyNewExperiences=true in update response")
	}
	if updateResp.Msg.Preferences.NotifyTransferUpdates != nil {
		t.Error("expected NotifyTransferUpdates to remain unset")
	}

	// Read back via Get and confirm same shape.
	getResp, err := service.GetCommunityNotificationPreferences(ctx, connect.NewRequest(
		&api.GetCommunityNotificationPreferencesRequest{CommunityId: "community-1"},
	))
	if err != nil {
		t.Fatalf("Get after update returned error: %v", err)
	}
	if getResp.Msg.Preferences.NotifyChats == nil || *getResp.Msg.Preferences.NotifyChats {
		t.Error("expected persisted NotifyChats=false")
	}
	if getResp.Msg.Preferences.NotifyTransferUpdates != nil {
		t.Error("unset toggles must remain unset after persist")
	}
}

func TestUpdateCommunityNotificationPreferences_IsIdempotentUpsert(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)
	userID := setupTestUser(t, sqlStorage, "carol@test.com", "Carol")
	ctx := createAuthenticatedContext(userID, "carol@test.com", models.Role_ROLE_USER)
	seedCommunityMembership(t, sqlStorage, "community-x", userID)

	disabled := false
	for i := range 3 {
		_, err := service.UpdateCommunityNotificationPreferences(ctx, connect.NewRequest(
			&api.UpdateCommunityNotificationPreferencesRequest{
				CommunityId: "community-x",
				Preferences: &api.CommunityNotificationPreferences{NotifyChats: &disabled},
			},
		))
		if err != nil {
			t.Fatalf("Update iteration %d failed: %v", i, err)
		}
	}

	// Exactly one row should exist for this user/community pair.
	rows, err := sqlStorage.QueryByFields(ctx, map[string]any{
		"user_id":      userID,
		"community_id": "community-x",
	}, &models.CommunityNotificationPreferences{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got := len(rows); got != 1 {
		t.Errorf("expected 1 row after repeated upsert, got %d", got)
	}
}

func TestNotificationPreferences_CrossUserAndCommunityIsolation(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)
	aliceID := setupTestUser(t, sqlStorage, "a@test.com", "A")
	bobID := setupTestUser(t, sqlStorage, "b@test.com", "B")

	aliceCtx := createAuthenticatedContext(aliceID, "a@test.com", models.Role_ROLE_USER)
	bobCtx := createAuthenticatedContext(bobID, "b@test.com", models.Role_ROLE_USER)

	seedCommunityMembership(t, sqlStorage, "community-1", aliceID)
	seedCommunityMembership(t, sqlStorage, "community-1", bobID)
	seedCommunityMembership(t, sqlStorage, "community-2", aliceID)

	disabled := false

	// Alice disables chats in community-1.
	_, err := service.UpdateCommunityNotificationPreferences(aliceCtx, connect.NewRequest(
		&api.UpdateCommunityNotificationPreferencesRequest{
			CommunityId: "community-1",
			Preferences: &api.CommunityNotificationPreferences{NotifyChats: &disabled},
		},
	))
	if err != nil {
		t.Fatalf("alice update: %v", err)
	}

	// Bob's view of community-1 must be unaffected.
	bobResp, err := service.GetCommunityNotificationPreferences(bobCtx, connect.NewRequest(
		&api.GetCommunityNotificationPreferencesRequest{CommunityId: "community-1"},
	))
	if err != nil {
		t.Fatalf("bob get: %v", err)
	}
	if bobResp.Msg.Preferences.NotifyChats != nil {
		t.Error("bob's prefs in community-1 should be untouched by alice's write")
	}

	// Alice's view of a different community must be unaffected.
	aliceOtherResp, err := service.GetCommunityNotificationPreferences(aliceCtx, connect.NewRequest(
		&api.GetCommunityNotificationPreferencesRequest{CommunityId: "community-2"},
	))
	if err != nil {
		t.Fatalf("alice get other community: %v", err)
	}
	if aliceOtherResp.Msg.Preferences.NotifyChats != nil {
		t.Error("alice's prefs in community-2 should be untouched")
	}
}

func TestStoredAPIPreferencesRoundTrip(t *testing.T) {
	on := true
	off := false
	original := &api.CommunityNotificationPreferences{
		NotifyNewRequests:         &on,
		NotifyNewExperiences:      nil, // unset
		NotifyGearShared:          &off,
		NotifyExperienceCompleted: &on,
		NotifyTransferUpdates:     nil, // unset
		NotifyRequestUpdates:      &off,
		NotifyExperienceRsvps:     nil, // unset
		NotifyPlanningUpdates:     &on,
		NotifyChats:               &off,
	}

	round := storedToAPIPreferences(apiToStoredPreferences(original))

	checks := []struct {
		name string
		got  *bool
		want *bool
	}{
		{"NotifyNewRequests", round.NotifyNewRequests, original.NotifyNewRequests},
		{"NotifyNewExperiences", round.NotifyNewExperiences, original.NotifyNewExperiences},
		{"NotifyGearShared", round.NotifyGearShared, original.NotifyGearShared},
		{"NotifyExperienceCompleted", round.NotifyExperienceCompleted, original.NotifyExperienceCompleted},
		{"NotifyTransferUpdates", round.NotifyTransferUpdates, original.NotifyTransferUpdates},
		{"NotifyRequestUpdates", round.NotifyRequestUpdates, original.NotifyRequestUpdates},
		{"NotifyExperienceRsvps", round.NotifyExperienceRsvps, original.NotifyExperienceRsvps},
		{"NotifyPlanningUpdates", round.NotifyPlanningUpdates, original.NotifyPlanningUpdates},
		{"NotifyChats", round.NotifyChats, original.NotifyChats},
	}
	for _, c := range checks {
		if (c.got == nil) != (c.want == nil) {
			t.Errorf("%s presence mismatch: got nil=%v want nil=%v", c.name, c.got == nil, c.want == nil)
			continue
		}
		if c.got != nil && *c.got != *c.want {
			t.Errorf("%s value mismatch: got %v want %v", c.name, *c.got, *c.want)
		}
	}
}
