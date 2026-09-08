package planning

import (
	"context"
	"errors"
	"testing"
)

func TestMockScopeLoader_RecordsCallsAndReturnsConfiguredResult(t *testing.T) {
	expResult := &ActiveScope{ConversationID: "conv-1", OwnerID: "owner-1"}
	m := &MockScopeLoader{Result: expResult}
	ctx := context.Background()

	scope1 := Scope{ExperienceID: "exp-1"}
	scope2 := Scope{ExperienceID: "exp-2"}

	got1, err := m.LoadActiveScope(ctx, scope1)
	if err != nil {
		t.Fatalf("LoadActiveScope(scope1): %v", err)
	}
	if got1 != expResult {
		t.Errorf("LoadActiveScope(scope1) result = %v, want %v", got1, expResult)
	}

	got2, err := m.LoadActiveScope(ctx, scope2)
	if err != nil {
		t.Fatalf("LoadActiveScope(scope2): %v", err)
	}
	if got2 != expResult {
		t.Errorf("LoadActiveScope(scope2) result = %v, want %v", got2, expResult)
	}

	if len(m.Calls) != 2 {
		t.Fatalf("Calls length = %d, want 2", len(m.Calls))
	}
	if m.Calls[0] != scope1 {
		t.Errorf("Calls[0] = %v, want %v", m.Calls[0], scope1)
	}
	if m.Calls[1] != scope2 {
		t.Errorf("Calls[1] = %v, want %v", m.Calls[1], scope2)
	}
}

func TestMockScopeLoader_ReturnsConfiguredError(t *testing.T) {
	wantErr := errors.New("not found")
	m := &MockScopeLoader{Err: wantErr}
	ctx := context.Background()

	_, err := m.LoadActiveScope(ctx, Scope{ExperienceID: "exp-1"})
	if err != wantErr {
		t.Errorf("LoadActiveScope error = %v, want %v", err, wantErr)
	}
	if len(m.Calls) != 1 {
		t.Errorf("Calls length = %d, want 1", len(m.Calls))
	}
}
