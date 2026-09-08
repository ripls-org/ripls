package planning

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	st, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return st
}

// TestExperienceScopeLoader_LoadActiveScope_ActiveExperience verifies that an active
// experience returns the correct ConversationID and OwnerID.
func TestExperienceScopeLoader_LoadActiveScope_ActiveExperience(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	exp := &models.Experience{
		OwnerId:        "owner-1",
		ConversationId: "conv-1",
		State:          models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	expID, err := st.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	loader := NewExperienceScopeLoader(st)
	scope, err := loader.LoadActiveScope(ctx, Scope{ExperienceID: expID})
	if err != nil {
		t.Fatalf("LoadActiveScope: %v", err)
	}
	if scope.ConversationID != "conv-1" {
		t.Errorf("ConversationID = %q, want %q", scope.ConversationID, "conv-1")
	}
	if scope.OwnerID != "owner-1" {
		t.Errorf("OwnerID = %q, want %q", scope.OwnerID, "owner-1")
	}
}

// TestExperienceScopeLoader_LoadActiveScope_TerminalState verifies that a completed or
// cancelled experience returns CodeFailedPrecondition.
func TestExperienceScopeLoader_LoadActiveScope_TerminalState(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	for _, state := range []models.ExperienceState{
		models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		models.ExperienceState_EXPERIENCE_STATE_CANCELLED,
	} {
		exp := &models.Experience{
			OwnerId: "owner-1",
			State:   state,
		}
		expID, err := st.Insert(ctx, exp)
		if err != nil {
			t.Fatalf("insert experience (%s): %v", state, err)
		}

		loader := NewExperienceScopeLoader(st)
		_, err = loader.LoadActiveScope(ctx, Scope{ExperienceID: expID})
		if err == nil {
			t.Errorf("LoadActiveScope(%s) expected error, got nil", state)
			continue
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("LoadActiveScope(%s) code = %v, want CodeFailedPrecondition", state, connect.CodeOf(err))
		}
	}
}

// TestExperienceScopeLoader_LoadActiveScope_NotFound verifies that a nonexistent
// experience returns CodeNotFound.
func TestExperienceScopeLoader_LoadActiveScope_NotFound(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	loader := NewExperienceScopeLoader(st)
	_, err := loader.LoadActiveScope(ctx, Scope{ExperienceID: "nonexistent-exp-id"})
	if err == nil {
		t.Fatal("LoadActiveScope expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeNotFound", connect.CodeOf(err))
	}
}

// TestRequestScopeLoader_LoadActiveScope_ActiveRequest verifies that an active
// request returns the correct ConversationID and OwnerID.
func TestRequestScopeLoader_LoadActiveScope_ActiveRequest(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	req := &models.Request{
		RequesterId:    "requester-1",
		ConversationId: "conv-req-1",
		State:          models.RequestState_REQUEST_STATE_ACTIVE,
	}
	reqID, err := st.Insert(ctx, req)
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}

	loader := NewRequestScopeLoader(st)
	scope, err := loader.LoadActiveScope(ctx, Scope{RequestID: reqID})
	if err != nil {
		t.Fatalf("LoadActiveScope: %v", err)
	}
	if scope.ConversationID != "conv-req-1" {
		t.Errorf("ConversationID = %q, want %q", scope.ConversationID, "conv-req-1")
	}
	if scope.OwnerID != "requester-1" {
		t.Errorf("OwnerID = %q, want %q", scope.OwnerID, "requester-1")
	}
}

// TestRequestScopeLoader_LoadActiveScope_TerminalState verifies that a fulfilled or
// cancelled request returns CodeFailedPrecondition.
func TestRequestScopeLoader_LoadActiveScope_TerminalState(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	for _, state := range []models.RequestState{
		models.RequestState_REQUEST_STATE_FULFILLED,
		models.RequestState_REQUEST_STATE_CANCELLED,
	} {
		req := &models.Request{
			RequesterId: "requester-1",
			State:       state,
		}
		reqID, err := st.Insert(ctx, req)
		if err != nil {
			t.Fatalf("insert request (%s): %v", state, err)
		}

		loader := NewRequestScopeLoader(st)
		_, err = loader.LoadActiveScope(ctx, Scope{RequestID: reqID})
		if err == nil {
			t.Errorf("LoadActiveScope(%s) expected error, got nil", state)
			continue
		}
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("LoadActiveScope(%s) code = %v, want CodeFailedPrecondition", state, connect.CodeOf(err))
		}
	}
}

// TestRequestScopeLoader_LoadActiveScope_NotFound verifies that a nonexistent
// request returns CodeNotFound.
func TestRequestScopeLoader_LoadActiveScope_NotFound(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()

	loader := NewRequestScopeLoader(st)
	_, err := loader.LoadActiveScope(ctx, Scope{RequestID: "nonexistent-req-id"})
	if err == nil {
		t.Fatal("LoadActiveScope expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("code = %v, want CodeNotFound", connect.CodeOf(err))
	}
}
