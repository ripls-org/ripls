// Package planning provides the shared library for planning needs and
// contributions, used by both the experience and request services.
package planning

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// Scope identifies the parent entity for a set of planning needs/contributions.
// Exactly one of ExperienceID or RequestID must be non-empty.
type Scope struct {
	ExperienceID string
	RequestID    string
}

// ScopeID returns the non-empty scope identifier.
func (s Scope) ScopeID() string {
	if s.ExperienceID != "" {
		return s.ExperienceID
	}
	return s.RequestID
}

// IsExperience reports whether this scope is for an experience.
func (s Scope) IsExperience() bool { return s.ExperienceID != "" }

// IsRequest reports whether this scope is for a request.
func (s Scope) IsRequest() bool { return s.RequestID != "" }

// scopeFieldName returns the proto field name for this scope ("experience_id" or "request_id").
func (s Scope) scopeFieldName() string {
	if s.IsExperience() {
		return "experience_id"
	}
	return "request_id"
}

// scopeKind returns a human-readable noun for the scope, for error messages.
func (s Scope) scopeKind() string {
	if s.IsExperience() {
		return "experience"
	}
	return "request"
}

// matchesContribution reports whether the contribution belongs to this scope.
func (s Scope) matchesContribution(c *models.PlanningContribution) bool {
	if s.IsExperience() {
		return c.GetExperienceId() == s.ExperienceID
	}
	return c.GetRequestId() == s.RequestID
}

// ActiveScope holds the resolved parent entity details needed for planning handlers.
type ActiveScope struct {
	ConversationID string
	OwnerID        string
}

// ScopeLoader is implemented by types that can load an ActiveScope for a given Scope.
type ScopeLoader interface {
	// LoadActiveScope loads the parent entity, verifies it is not in a terminal
	// state, and returns the details needed by planning handlers.
	// Returns CodeNotFound if the entity does not exist, CodeFailedPrecondition
	// if the entity is in a terminal state.
	LoadActiveScope(ctx context.Context, scope Scope) (*ActiveScope, error)
}

// experienceScopeLoader loads an Experience as an ActiveScope.
type experienceScopeLoader struct {
	st *storage.ProtoSQLStorage
}

// NewExperienceScopeLoader creates a ScopeLoader backed by the Experience storage table.
func NewExperienceScopeLoader(st *storage.ProtoSQLStorage) ScopeLoader {
	return &experienceScopeLoader{st: st}
}

// LoadActiveScope loads an Experience, validates its state, and returns the ActiveScope.
func (l *experienceScopeLoader) LoadActiveScope(ctx context.Context, scope Scope) (*ActiveScope, error) {
	exp := &models.Experience{}
	if err := l.st.GetByID(ctx, scope.ExperienceID, exp); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if exp.State == models.ExperienceState_EXPERIENCE_STATE_COMPLETED ||
		exp.State == models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("experience is in a terminal state (%s); no changes allowed", exp.State))
	}
	return &ActiveScope{ConversationID: exp.ConversationId, OwnerID: exp.OwnerId}, nil
}

// requestScopeLoader loads a Request as an ActiveScope.
type requestScopeLoader struct {
	st *storage.ProtoSQLStorage
}

// NewRequestScopeLoader creates a ScopeLoader backed by the Request storage table.
func NewRequestScopeLoader(st *storage.ProtoSQLStorage) ScopeLoader {
	return &requestScopeLoader{st: st}
}

// LoadActiveScope loads a Request, validates its state, and returns the ActiveScope.
func (l *requestScopeLoader) LoadActiveScope(ctx context.Context, scope Scope) (*ActiveScope, error) {
	req := &models.Request{}
	if err := l.st.GetByID(ctx, scope.RequestID, req); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if req.State == models.RequestState_REQUEST_STATE_FULFILLED ||
		req.State == models.RequestState_REQUEST_STATE_CANCELLED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("request is in a terminal state (%s); no changes allowed", req.State))
	}
	return &ActiveScope{ConversationID: req.ConversationId, OwnerID: req.RequesterId}, nil
}
