package impact_metrics

// authorization.go — Authorization helpers for impact metrics RPC handlers.

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/storage"
)

// authorizeExperienceDraft verifies the caller is the experience host or a confirmed attendee.
// Returns connect.CodePermissionDenied if the caller is not authorized, or
// connect.CodeNotFound if the experience does not exist.
func authorizeExperienceDraft(ctx context.Context, experienceID string, s *storage.ProtoSQLStorage) error {
	authInfo, authErr := auth.RequireAuth(ctx)
	if authErr != nil {
		return authErr
	}

	exp := &models.Experience{}
	if getErr := s.GetByID(ctx, experienceID, exp); getErr != nil {
		return connect.NewError(connect.CodeNotFound, errors.New("experience not found"))
	}

	if exp.OwnerId == authInfo.UserID {
		return nil
	}

	// Check confirmed RSVPs.
	rsvps, queryErr := storage.QueryByFields[*models.ExperienceRSVP](s, ctx, map[string]any{
		"experience_id": experienceID,
		"user_id":       authInfo.UserID,
	})
	if queryErr != nil {
		return connecterr.Internal(ctx, "authorizeExperienceDraft", queryErr)
	}
	for _, r := range rsvps {
		if rsvpstate.IsGoing(r.GetIntention()) {
			return nil
		}
	}

	return connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
}

// authorizeRequestDraft verifies the caller is the request creator or a confirmed helper.
// Returns connect.CodePermissionDenied if the caller is not authorized, or
// connect.CodeNotFound if the request does not exist.
func authorizeRequestDraft(ctx context.Context, requestID string, s *storage.ProtoSQLStorage) error {
	authInfo, authErr := auth.RequireAuth(ctx)
	if authErr != nil {
		return authErr
	}

	req := &models.Request{}
	if getErr := s.GetByID(ctx, requestID, req); getErr != nil {
		return connect.NewError(connect.CodeNotFound, errors.New("request not found"))
	}

	if req.RequesterId == authInfo.UserID {
		return nil
	}

	// Check confirmed helper IDs.
	for _, helperID := range req.ConfirmedHelperIds {
		if helperID == authInfo.UserID {
			return nil
		}
	}

	// Check active offers.
	offers, queryErr := storage.QueryByFields[*models.RequestOffer](s, ctx, map[string]any{
		"request_id": requestID,
		"user_id":    authInfo.UserID,
	})
	if queryErr != nil {
		return connecterr.Internal(ctx, "authorizeRequestDraft", queryErr)
	}
	for _, o := range offers {
		if !o.Withdrawn {
			return nil
		}
	}

	return connect.NewError(connect.CodePermissionDenied, errors.New("not authorized"))
}
