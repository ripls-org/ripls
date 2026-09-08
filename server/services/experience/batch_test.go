package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestBatchAddExperienceNeeds_HappyPath verifies that each need in a batch
// produces its own chat message. Items without a note emit a system message
// keyed by need ID (so they don't coalesce with each other); items with a
// note emit a user message.
func TestBatchAddExperienceNeeds_HappyPath(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "batch-needs-owner"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)

	note := "something nice"
	resp, err := service.BatchAddExperienceNeeds(ownerCtx, connect.NewRequest(&api.BatchAddExperienceNeedsRequest{
		ExperienceId: expID,
		Items: []*api.BatchNeedItem{
			{Name: "Wine", Slots: 2},
			{Name: "Cheese", Note: &note, Slots: 1},
			{Name: "Bread", Slots: 1},
		},
	}))
	if err != nil {
		t.Fatalf("BatchAddExperienceNeeds: %v", err)
	}

	if got := len(resp.Msg.Needs); got != 3 {
		t.Errorf("returned %d needs, want 3", got)
	}

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	newSys := afterSys[len(beforeSys):]
	// Wine and Bread have no note → one system message each (different coalesce keys).
	// Cheese has a note → user message (not counted as a system message).
	if len(newSys) != 2 {
		t.Errorf("emitted %d system messages, want 2", len(newSys))
	}
	for _, sm := range newSys {
		if sm.Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED {
			t.Errorf("action = %v, want PLANNING_NEED_ADDED", sm.Action)
		}
	}
}

// TestBatchAddExperienceNeeds_SingleItem verifies single-item batch still works.
func TestBatchAddExperienceNeeds_SingleItem(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "batch-needs-single"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)

	resp, err := service.BatchAddExperienceNeeds(ownerCtx, connect.NewRequest(&api.BatchAddExperienceNeedsRequest{
		ExperienceId: expID,
		Items:        []*api.BatchNeedItem{{Name: "Plates", Slots: 1}},
	}))
	if err != nil {
		t.Fatalf("BatchAddExperienceNeeds single: %v", err)
	}
	if len(resp.Msg.Needs) != 1 {
		t.Errorf("returned %d needs, want 1", len(resp.Msg.Needs))
	}

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	if got := len(afterSys) - len(beforeSys); got != 1 {
		t.Errorf("emitted %d messages, want 1", got)
	}
}

// TestBatchAddExperienceNeeds_EmptyItems rejects an empty batch.
func TestBatchAddExperienceNeeds_EmptyItems(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)
	ownerID := "batch-needs-empty"
	expID, _ := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	_, err := service.BatchAddExperienceNeeds(ownerCtx, connect.NewRequest(&api.BatchAddExperienceNeedsRequest{
		ExperienceId: expID,
	}))
	if err == nil {
		t.Fatal("expected error for empty items, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// TestBatchAddExperienceNeeds_BlankName rejects an item with a blank name.
func TestBatchAddExperienceNeeds_BlankName(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)
	ownerID := "batch-needs-blank"
	expID, _ := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	_, err := service.BatchAddExperienceNeeds(ownerCtx, connect.NewRequest(&api.BatchAddExperienceNeedsRequest{
		ExperienceId: expID,
		Items:        []*api.BatchNeedItem{{Name: "Good item"}, {Name: ""}},
	}))
	if err == nil {
		t.Fatal("expected error for blank item name, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

// TestBatchAddExperienceNeeds_NonParticipantRejected verifies non-participants cannot add needs.
func TestBatchAddExperienceNeeds_NonParticipantRejected(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)
	ownerID := "batch-needs-owner-np"
	expID, _ := setupSharedExperience(t, service, testStorage, ownerID)

	outsiderID := "batch-needs-outsider"
	createTestUser(t, testStorage, outsiderID, outsiderID+"@example.com", "Outsider")
	outsiderCtx := createAuthenticatedContext(outsiderID, outsiderID+"@example.com", models.Role_ROLE_USER)

	_, err := service.BatchAddExperienceNeeds(outsiderCtx, connect.NewRequest(&api.BatchAddExperienceNeedsRequest{
		ExperienceId: expID,
		Items:        []*api.BatchNeedItem{{Name: "Sneaky item", Slots: 1}},
	}))
	if err == nil {
		t.Fatal("expected error for non-participant, got nil")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("error code = %v, want PermissionDenied", connect.CodeOf(err))
	}
}

// TestBatchAddExperienceContributions_HappyPath verifies multiple contributions are created
// with a single batch system message.
func TestBatchAddExperienceContributions_HappyPath(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)

	ownerID := "batch-contrib-owner"
	expID, conversationID := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	beforeSys := getSystemMessagesForConversation(t, testStorage, conversationID)

	desc := "homemade"
	resp, err := service.BatchAddExperienceContributions(ownerCtx, connect.NewRequest(&api.BatchAddExperienceContributionsRequest{
		ExperienceId: expID,
		Items: []*api.BatchContributionItem{
			{Title: "Guacamole", Description: &desc},
			{Title: "Chips"},
		},
	}))
	if err != nil {
		t.Fatalf("BatchAddExperienceContributions: %v", err)
	}

	if got := len(resp.Msg.Contributions); got != 2 {
		t.Errorf("returned %d contributions, want 2", got)
	}

	afterSys := getSystemMessagesForConversation(t, testStorage, conversationID)
	newSys := afterSys[len(beforeSys):]
	// Guacamole has a description → user message (not counted here). Chips has none → system message.
	if len(newSys) != 1 {
		t.Errorf("emitted %d system messages, want 1", len(newSys))
	} else if newSys[0].Action != models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED {
		t.Errorf("action = %v, want PLANNING_CONTRIBUTION_ADDED", newSys[0].Action)
	}
}

// TestBatchAddExperienceContributions_BlankTitle rejects an item with a blank title.
func TestBatchAddExperienceContributions_BlankTitle(t *testing.T) {
	service, testStorage := setupTestServiceWithWriter(t)
	ownerID := "batch-contrib-blank"
	expID, _ := setupSharedExperience(t, service, testStorage, ownerID)
	ownerCtx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)

	_, err := service.BatchAddExperienceContributions(ownerCtx, connect.NewRequest(&api.BatchAddExperienceContributionsRequest{
		ExperienceId: expID,
		Items:        []*api.BatchContributionItem{{Title: "Valid"}, {Title: ""}},
	}))
	if err == nil {
		t.Fatal("expected error for blank title, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}
