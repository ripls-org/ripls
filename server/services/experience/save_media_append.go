package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// isAppendOnlyMediaUpdate reports whether [req] is an "append-only" SaveExperience
// update — i.e. the caller is only adding media IDs to [stored] and not
// modifying any other field. Returns a permission-denied error when the
// request attempts to change something other than the media list, so the
// non-owner carve-out in SaveExperience can fall through to the standard
// owner-only rejection with a clear message.
//
// "Append-only" means:
//   - No text, time, or location change (detected by the flags already
//     computed in SaveExperience).
//   - MaxParticipants is unset (0) or equal to the stored value.
//   - SourceUrl is unset.
//   - SocialContext and Metadata are unset.
//   - Every stored MediaId is preserved in the request (the request is a
//     superset of stored).
//   - At least one new media ID is being added (otherwise there is nothing
//     to do and the call should not have been made).
func isAppendOnlyMediaUpdate(
	req *api.SaveExperienceRequest,
	stored *models.Experience,
	textChanged, timeChanged, locationChanged bool,
) (bool, error) {
	if textChanged || timeChanged || locationChanged {
		return false, nil
	}
	if req.MaxParticipants != 0 && req.MaxParticipants != stored.MaxParticipants {
		return false, nil
	}
	if req.SourceUrl != nil {
		return false, nil
	}
	if req.SocialContext != nil || req.Metadata != nil {
		return false, nil
	}

	// Verify mediaIds is a strict superset of stored.MediaIds. We require
	// the relative order of the stored IDs to be preserved so a non-owner
	// cannot reorder existing media — only append.
	storedIDs := stored.MediaIds
	reqIDs := req.MediaIds
	if len(reqIDs) <= len(storedIDs) {
		return false, nil
	}
	for i, id := range storedIDs {
		if i >= len(reqIDs) || reqIDs[i] != id {
			return false, nil
		}
	}
	return true, nil
}

// callerMayAddExperienceMedia returns nil when [userID] is permitted to
// append media to [experienceID] as a non-owner. Today that means the
// caller must be a member of at least one community the experience is
// shared with. Other failures (storage errors) bubble up as internal
// connect errors so the client surfaces a generic "try again" message
// rather than a misleading permission-denied.
func (s *Service) callerMayAddExperienceMedia(
	ctx context.Context,
	experienceID string,
	userID string,
) error {
	communityExperiences, err := s.storage.QueryByField(
		ctx, "experience_id", experienceID, &models.CommunityExperience{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "SaveExperience",
			fmt.Errorf("failed to load shared communities: %w", err),
			"experience_id", experienceID)
	}
	if len(communityExperiences) == 0 {
		return connecterr.UserVisible(
			ctx,
			connect.CodePermissionDenied,
			"experience_owner_required_for_update",
			"only the owner can update this event",
			nil,
		)
	}
	communityIDs := make([]string, 0, len(communityExperiences))
	for _, m := range communityExperiences {
		communityIDs = append(communityIDs, m.(*models.CommunityExperience).CommunityId)
	}
	memberships, err := s.storage.QueryByFieldIn(
		ctx, "community_id", communityIDs, &models.CommunityUser{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "SaveExperience",
			fmt.Errorf("failed to load community memberships: %w", err),
			"experience_id", experienceID)
	}
	for _, m := range memberships {
		if m.(*models.CommunityUser).UserId == userID {
			return nil
		}
	}
	return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "media_add_requires_shared_community", "only members of a shared community can add media", nil)
}
