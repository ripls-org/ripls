package request

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// isAppendOnlyRequestMediaUpdate reports whether [req] is an "append-only"
// UpdateRequest call — i.e. the caller is only adding media IDs to [stored]
// and not modifying any other field. Mirrors the carve-out applied to
// SaveExperience for the media-carousel "Add media" flow.
//
// "Append-only" means:
//   - Title is unset or equal to the stored value.
//   - Description is unset or equal to the stored value.
//   - LocationId is unset or equal to the stored value.
//   - SocialContext is unset.
//   - Every stored MediaId is preserved in the request in its original
//     order (the request is a strict superset of stored.MediaIds), and at
//     least one new media ID is being added.
func isAppendOnlyRequestMediaUpdate(
	req *api.UpdateRequestRequest,
	stored *models.Request,
) bool {
	if req.Title != "" && req.Title != stored.Title {
		return false
	}
	if req.Description != "" && req.Description != stored.Description {
		return false
	}
	if req.LocationId != "" && req.LocationId != stored.LocationId {
		return false
	}
	if req.SocialContext != nil {
		return false
	}

	storedIDs := stored.MediaIds
	reqIDs := req.MediaIds
	if len(reqIDs) <= len(storedIDs) {
		return false
	}
	for i, id := range storedIDs {
		if i >= len(reqIDs) || reqIDs[i] != id {
			return false
		}
	}
	return true
}

// callerMayAddRequestMedia returns nil when [userID] is permitted to append
// media to [requestID] as a non-requester. Today that means the caller must
// be a member of at least one community the request is shared with.
func (s *Service) callerMayAddRequestMedia(
	ctx context.Context,
	requestID string,
	userID string,
) error {
	communityRequests, err := s.storage.QueryByField(
		ctx, "request_id", requestID, &models.CommunityRequest{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "UpdateRequest",
			fmt.Errorf("failed to load shared communities: %w", err),
			"target_request_id", requestID)
	}
	if len(communityRequests) == 0 {
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "request_creator_required_for_update", "only the requester can update this request", nil)
	}
	communityIDs := make([]string, 0, len(communityRequests))
	for _, m := range communityRequests {
		communityIDs = append(communityIDs, m.(*models.CommunityRequest).CommunityId)
	}
	memberships, err := s.storage.QueryByFieldIn(
		ctx, "community_id", communityIDs, &models.CommunityUser{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "UpdateRequest",
			fmt.Errorf("failed to load community memberships: %w", err),
			"target_request_id", requestID)
	}
	for _, m := range memberships {
		if m.(*models.CommunityUser).UserId == userID {
			return nil
		}
	}
	return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "media_add_requires_shared_community", "only members of a shared community can add media", nil)
}
