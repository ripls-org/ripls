package user

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GetUserNotificationPreferences reads the calling user's user-scoped
// notification preferences. Returns an empty preferences message (all
// fields unset, semantically "all on") when no row exists yet.
func (s *Service) GetUserNotificationPreferences(
	ctx context.Context,
	_ *connect.Request[api.GetUserNotificationPreferencesRequest],
) (*connect.Response[api.GetUserNotificationPreferencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserNotificationPreferences",
		"user_id", authInfo.UserID,
	)

	prefs, err := loadUserNotificationPreferences(ctx, s.storage, authInfo.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load preferences", "error", err)
		return nil, connecterr.Internal(ctx, "GetUserNotificationPreferences", err)
	}

	return connect.NewResponse(&api.GetUserNotificationPreferencesResponse{
		Preferences: storedToAPIUserPreferences(prefs),
	}), nil
}

// UpdateUserNotificationPreferences writes the calling user's
// user-scoped notification preferences. Fields explicitly set on the
// request overwrite stored values; fields left unset preserve their
// existing value (so the client can partially update).
func (s *Service) UpdateUserNotificationPreferences(
	ctx context.Context,
	req *connect.Request[api.UpdateUserNotificationPreferencesRequest],
) (*connect.Response[api.UpdateUserNotificationPreferencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "UpdateUserNotificationPreferences",
		"user_id", authInfo.UserID,
	)

	now := clock.UnixSec(ctx)
	existing, err := loadUserNotificationPreferences(ctx, s.storage, authInfo.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to load preferences", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateUserNotificationPreferences", err)
	}

	if existing == nil {
		merged := apiToStoredUserPreferences(req.Msg.Preferences)
		merged.UserId = authInfo.UserID
		merged.CreatedAtUnixSec = now
		merged.UpdatedAtUnixSec = now
		if _, err := s.storage.Insert(ctx, merged); err != nil {
			logger.ErrorContext(ctx, "failed to insert preferences", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateUserNotificationPreferences", err)
		}
		return connect.NewResponse(&api.UpdateUserNotificationPreferencesResponse{
			Preferences: storedToAPIUserPreferences(merged),
		}), nil
	}

	applyPreferenceUpdates(existing, req.Msg.Preferences)
	existing.UpdatedAtUnixSec = now
	if err := s.storage.Update(ctx, existing); err != nil {
		logger.ErrorContext(ctx, "failed to update preferences", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateUserNotificationPreferences", err)
	}

	return connect.NewResponse(&api.UpdateUserNotificationPreferencesResponse{
		Preferences: storedToAPIUserPreferences(existing),
	}), nil
}

// loadUserNotificationPreferences returns the row for the given user
// or nil if no row exists yet. Treats "not found" as not-an-error so
// callers can fall back to the empty-preferences semantic. Returns
// other storage errors verbatim.
func loadUserNotificationPreferences(
	ctx context.Context, s *storage.ProtoSQLStorage, userID string,
) (*models.UserNotificationPreferences, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	rows, err := s.QueryByField(ctx, "user_id", userID, &models.UserNotificationPreferences{})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0].(*models.UserNotificationPreferences), nil
}

// LoadUserNotificationPreferences is a package-public version of the
// internal loader, exposed for reconcilers/dispatchers in
// server/jobs/scheduled_notifications that need to gate on user-scoped
// preferences. Returns nil when no row exists (caller treats unset as
// on).
func LoadUserNotificationPreferences(
	ctx context.Context, s *storage.ProtoSQLStorage, userID string,
) (*models.UserNotificationPreferences, error) {
	prefs, err := loadUserNotificationPreferences(ctx, s, userID)
	if err != nil && !errors.Is(err, storage.ErrRecordNotFound) {
		return nil, err
	}
	return prefs, nil
}

// applyPreferenceUpdates copies only the set fields of `incoming`
// over `target`. This preserves the partial-update contract: fields
// the caller didn't include keep their existing values.
func applyPreferenceUpdates(target *models.UserNotificationPreferences, incoming *api.UserNotificationPreferences) {
	if incoming == nil {
		return
	}
	if incoming.NotifyLoanReturnReminders != nil {
		v := *incoming.NotifyLoanReturnReminders
		target.NotifyLoanReturnReminders = &v
	}
	if incoming.NotifyEventClosePrompts != nil {
		v := *incoming.NotifyEventClosePrompts
		target.NotifyEventClosePrompts = &v
	}
}

// storedToAPIUserPreferences converts a storage preferences message
// to its API representation, preserving the unset-vs-explicit-false
// distinction.
func storedToAPIUserPreferences(p *models.UserNotificationPreferences) *api.UserNotificationPreferences {
	out := &api.UserNotificationPreferences{}
	if p == nil {
		return out
	}
	out.NotifyLoanReturnReminders = copyUserBoolPtr(p.NotifyLoanReturnReminders)
	out.NotifyEventClosePrompts = copyUserBoolPtr(p.NotifyEventClosePrompts)
	return out
}

// apiToStoredUserPreferences converts an API preferences message to
// its storage representation, preserving the unset-vs-explicit-false
// distinction.
func apiToStoredUserPreferences(p *api.UserNotificationPreferences) *models.UserNotificationPreferences {
	out := &models.UserNotificationPreferences{}
	if p == nil {
		return out
	}
	out.NotifyLoanReturnReminders = copyUserBoolPtr(p.NotifyLoanReturnReminders)
	out.NotifyEventClosePrompts = copyUserBoolPtr(p.NotifyEventClosePrompts)
	return out
}

func copyUserBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
