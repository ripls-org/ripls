package planning

import (
	"context"
)

// MockScopeLoader is a test implementation of ScopeLoader.
type MockScopeLoader struct {
	// Result is returned from LoadActiveScope on success.
	Result *ActiveScope
	// Err is returned from LoadActiveScope when non-nil.
	Err error
	// Calls records every scope passed to LoadActiveScope.
	Calls []Scope
}

// LoadActiveScope records the call and returns the configured Result or Err.
func (m *MockScopeLoader) LoadActiveScope(_ context.Context, scope Scope) (*ActiveScope, error) {
	m.Calls = append(m.Calls, scope)
	return m.Result, m.Err
}
