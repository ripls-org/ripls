package experience

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupExperienceForUndo creates an experience, shares it with a
// community, marks it in process, and completes it. Returns the
// community_event_id the client would use to call
// UndoCompleteExperience. Every undo test needs this shape so the
// EXPERIENCE_COMPLETED event emission has a community to target.
func setupExperienceForUndo(t *testing.T, service *Service, testStorage *storage.ProtoSQLStorage, ownerID, ownerEmail string) (expID, communityEventID string) {
	t.Helper()
	ownerCtx := createAuthenticatedContext(ownerID, ownerEmail, models.Role_ROLE_USER)

	createResp, err := service.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "Undo Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID = createResp.Msg.Experience.Id

	communityID := createTestCommunity(t, testStorage, "Undo Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)
	shareExperienceForTest(t, service, ownerCtx, expID, communityID)

	if _, err := service.MarkExperienceInProcess(ownerCtx, connect.NewRequest(&api.MarkExperienceInProcessRequest{
		ExperienceId: expID,
	})); err != nil {
		t.Fatalf("MarkExperienceInProcess failed: %v", err)
	}

	completeResp, err := service.CompleteExperience(ownerCtx, connect.NewRequest(&api.CompleteExperienceRequest{
		ExperienceId: expID,
	}))
	if err != nil {
		t.Fatalf("CompleteExperience failed: %v", err)
	}

	communityEventID = completeResp.Msg.CommunityEventId
	if communityEventID == "" {
		t.Fatal("CompleteExperienceResponse missing community_event_id")
	}
	return expID, communityEventID
}

// TestUndoCompleteExperience_Success verifies the happy path: owner
// marks an experience complete, then undoes the completion. State
// reverts, completion chat message soft-deletes, retraction event
// emitted.
func TestUndoCompleteExperience_Success(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	expID, communityEventID := setupExperienceForUndo(t, service, testStorage, "user123", "test@example.com")
	ownerCtx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Sanity: experience is COMPLETED.
	stored := &models.Experience{}
	if err := testStorage.GetByID(ownerCtx, expID, stored); err != nil {
		t.Fatalf("load experience: %v", err)
	}
	if stored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED {
		t.Fatalf("pre-undo state: want COMPLETED, got %s", stored.State)
	}

	if _, err := service.UndoCompleteExperience(ownerCtx, connect.NewRequest(&api.UndoCompleteExperienceRequest{
		CommunityEventId: communityEventID,
	})); err != nil {
		t.Fatalf("UndoCompleteExperience failed: %v", err)
	}

	// State reverts to IN_PROCESS (the prior state captured in UndoData).
	if err := testStorage.GetByID(ownerCtx, expID, stored); err != nil {
		t.Fatalf("post-undo load: %v", err)
	}
	if stored.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS {
		t.Errorf("post-undo state: want IN_PROCESS, got %s", stored.State)
	}
	if stored.CompletedAtUnixSec != nil {
		t.Errorf("post-undo CompletedAtUnixSec: want nil, got %v", *stored.CompletedAtUnixSec)
	}

	// Retraction event exists.
	events, err := testStorage.QueryByField(ownerCtx, "experience_id", expID, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	var foundRetraction bool
	for _, e := range events {
		if e.(*models.CommunityEvent).EventType ==
			models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE {
			foundRetraction = true
			break
		}
	}
	if !foundRetraction {
		t.Error("expected EXPERIENCE_COMPLETED_UNDONE retraction event; none found")
	}
}

// TestUndoCompleteExperience_NotActor verifies only the actor can
// undo their own completion.
func TestUndoCompleteExperience_NotActor(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "owner123", "owner@example.com", "Owner")
	createTestUser(t, testStorage, "other456", "other@example.com", "Other")
	_, communityEventID := setupExperienceForUndo(t, service, testStorage, "owner123", "owner@example.com")

	otherCtx := createAuthenticatedContext("other456", "other@example.com", models.Role_ROLE_USER)
	_, err := service.UndoCompleteExperience(otherCtx, connect.NewRequest(&api.UndoCompleteExperienceRequest{
		CommunityEventId: communityEventID,
	}))
	if err == nil {
		t.Fatal("expected error from non-actor undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_NOT_ACTOR {
		t.Errorf("reason: want NOT_ACTOR, got %s", reason)
	}
}

// TestUndoCompleteExperience_AlreadyUndone verifies the second undo
// call rejects with ALREADY_UNDONE.
func TestUndoCompleteExperience_AlreadyUndone(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")
	_, communityEventID := setupExperienceForUndo(t, service, testStorage, "user123", "test@example.com")
	ownerCtx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	undoReq := connect.NewRequest(&api.UndoCompleteExperienceRequest{
		CommunityEventId: communityEventID,
	})

	if _, err := service.UndoCompleteExperience(ownerCtx, undoReq); err != nil {
		t.Fatalf("first undo failed: %v", err)
	}

	_, err := service.UndoCompleteExperience(ownerCtx, undoReq)
	if err == nil {
		t.Fatal("expected error from second undo, got nil")
	}
	cerr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if reason := extractUndoFailureReason(cerr); reason != api.UndoFailureReason_UNDO_FAILURE_REASON_ALREADY_UNDONE {
		t.Errorf("reason: want ALREADY_UNDONE, got %s", reason)
	}
}

// extractUndoFailureReason unpacks UndoErrorDetail from a Connect error's
// details and returns its Reason.
func extractUndoFailureReason(cerr *connect.Error) api.UndoFailureReason {
	for _, d := range cerr.Details() {
		val, err := d.Value()
		if err != nil {
			continue
		}
		if detail, ok := val.(*api.UndoErrorDetail); ok {
			return detail.Reason
		}
	}
	return api.UndoFailureReason_UNDO_FAILURE_REASON_UNSPECIFIED
}
