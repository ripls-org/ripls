package community

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GetCommunityNotificationPreferences reads the calling user's per-category
// notification preferences for a community. Missing toggles in the response
// mean the category is enabled — clients must treat unset as "on".
func (s *Service) GetCommunityNotificationPreferences(
	ctx context.Context,
	req *connect.Request[api.GetCommunityNotificationPreferencesRequest],
) (*connect.Response[api.GetCommunityNotificationPreferencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	communityID := req.Msg.CommunityId
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, communityID, authInfo.UserID); err != nil {
		return nil, err
	}

	prefs, err := s.loadNotificationPreferences(ctx, authInfo.UserID, communityID)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to load notification preferences",
			"user_id", authInfo.UserID,
			"community_id", communityID,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetCommunityNotificationPreferences", err, "detail", "load preferences")
	}

	return connect.NewResponse(&api.GetCommunityNotificationPreferencesResponse{
		Preferences: storedToAPIPreferences(prefs),
	}), nil
}

// UpdateCommunityNotificationPreferences upserts the calling user's
// per-category notification preferences for a community.
func (s *Service) UpdateCommunityNotificationPreferences(
	ctx context.Context,
	req *connect.Request[api.UpdateCommunityNotificationPreferencesRequest],
) (*connect.Response[api.UpdateCommunityNotificationPreferencesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	communityID := req.Msg.CommunityId
	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("community_id is required"))
	}

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, communityID, authInfo.UserID); err != nil {
		return nil, err
	}

	existing, err := s.loadNotificationPreferences(ctx, authInfo.UserID, communityID)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to load existing preferences",
			"user_id", authInfo.UserID,
			"community_id", communityID,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "UpdateCommunityNotificationPreferences", err, "detail", "load preferences")
	}

	now := clock.UnixSec(ctx)
	updated := apiToStoredPreferences(req.Msg.Preferences)
	updated.CommunityId = communityID
	updated.UserId = authInfo.UserID
	updated.UpdatedAtUnixSec = now

	if existing == nil {
		updated.Id = uuid.NewString()
		updated.CreatedAtUnixSec = now
		if _, err := s.storage.Insert(ctx, updated); err != nil {
			return nil, connecterr.Internal(ctx, "UpdateCommunityNotificationPreferences", err, "detail", "insert preferences")
		}
	} else {
		updated.Id = existing.Id
		updated.CreatedAtUnixSec = existing.CreatedAtUnixSec
		if err := s.storage.Update(ctx, updated); err != nil {
			return nil, connecterr.Internal(ctx, "UpdateCommunityNotificationPreferences", err, "detail", "update preferences")
		}
	}

	return connect.NewResponse(&api.UpdateCommunityNotificationPreferencesResponse{
		Preferences: storedToAPIPreferences(updated),
	}), nil
}

// loadNotificationPreferences returns the stored row for (user, community)
// or nil if no row exists. Callers treat nil as "all categories on".
func (s *Service) loadNotificationPreferences(
	ctx context.Context,
	userID, communityID string,
) (*models.CommunityNotificationPreferences, error) {
	rows, err := s.storage.QueryByFields(ctx, map[string]any{
		"user_id":      userID,
		"community_id": communityID,
	}, &models.CommunityNotificationPreferences{})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0].(*models.CommunityNotificationPreferences), nil
}

// storedToAPIPreferences converts a stored preferences row to its API
// representation. nil input yields an empty API message (all unset = all on).
func storedToAPIPreferences(p *models.CommunityNotificationPreferences) *api.CommunityNotificationPreferences {
	out := &api.CommunityNotificationPreferences{}
	if p == nil {
		return out
	}
	out.NotifyNewRequests = copyBoolPtr(p.NotifyNewRequests)
	out.NotifyNewExperiences = copyBoolPtr(p.NotifyNewExperiences)
	out.NotifyGearShared = copyBoolPtr(p.NotifyGearShared)
	out.NotifyExperienceCompleted = copyBoolPtr(p.NotifyExperienceCompleted)
	out.NotifyTransferUpdates = copyBoolPtr(p.NotifyTransferUpdates)
	out.NotifyRequestUpdates = copyBoolPtr(p.NotifyRequestUpdates)
	out.NotifyExperienceRsvps = copyBoolPtr(p.NotifyExperienceRsvps)
	out.NotifyPlanningUpdates = copyBoolPtr(p.NotifyPlanningUpdates)
	out.NotifyChats = copyBoolPtr(p.NotifyChats)
	out.NotifyNewMembers = copyBoolPtr(p.NotifyNewMembers)
	out.NotifyEventReminders = copyBoolPtr(p.NotifyEventReminders)
	out.NotifyRequestFollowupPrompts = copyBoolPtr(p.NotifyRequestFollowupPrompts)
	return out
}

// apiToStoredPreferences converts an API preferences message to its storage
// representation, preserving the unset-vs-explicit-false distinction.
func apiToStoredPreferences(p *api.CommunityNotificationPreferences) *models.CommunityNotificationPreferences {
	out := &models.CommunityNotificationPreferences{}
	if p == nil {
		return out
	}
	out.NotifyNewRequests = copyBoolPtr(p.NotifyNewRequests)
	out.NotifyNewExperiences = copyBoolPtr(p.NotifyNewExperiences)
	out.NotifyGearShared = copyBoolPtr(p.NotifyGearShared)
	out.NotifyExperienceCompleted = copyBoolPtr(p.NotifyExperienceCompleted)
	out.NotifyTransferUpdates = copyBoolPtr(p.NotifyTransferUpdates)
	out.NotifyRequestUpdates = copyBoolPtr(p.NotifyRequestUpdates)
	out.NotifyExperienceRsvps = copyBoolPtr(p.NotifyExperienceRsvps)
	out.NotifyPlanningUpdates = copyBoolPtr(p.NotifyPlanningUpdates)
	out.NotifyChats = copyBoolPtr(p.NotifyChats)
	out.NotifyNewMembers = copyBoolPtr(p.NotifyNewMembers)
	out.NotifyEventReminders = copyBoolPtr(p.NotifyEventReminders)
	out.NotifyRequestFollowupPrompts = copyBoolPtr(p.NotifyRequestFollowupPrompts)
	return out
}

func copyBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
