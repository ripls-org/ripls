package gear

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// isAppendOnlyGearMediaUpdate reports whether [req] is an "append-only"
// SaveGear update — i.e. the caller is only adding media IDs to [stored]
// and not modifying any other field. Mirrors the carve-out applied to
// SaveExperience / UpdateRequest for the media-carousel "Add media" flow.
//
// "Append-only" means:
//   - Name, Description, LocationId, SourceUrl, GenerationMode are unset
//     (nil) or equal to the stored value (for the *string fields, a nil
//     pointer means "leave unchanged" — anything else, even pointing to
//     "", is a change request).
//   - Metadata is unset.
//   - Every stored MediaId is preserved in the request in its original
//     order (the request is a strict superset of stored.MediaIds), and at
//     least one new media ID is being added.
func isAppendOnlyGearMediaUpdate(
	req *api.SaveGearRequest,
	stored *models.Gear,
) bool {
	if req.Name != nil && *req.Name != stored.Name {
		return false
	}
	if req.Description != nil && *req.Description != stored.Description {
		return false
	}
	if req.LocationId != nil && *req.LocationId != stored.LocationId {
		return false
	}
	if req.SourceUrl != nil {
		// SourceUrl is rarely set on update and conceptually owner-only.
		// Treat any presence as a non-append change to keep this simple.
		return false
	}
	if req.GenerationMode != nil {
		return false
	}
	if req.Metadata != nil {
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

// callerMayAddGearMedia returns nil when [userID] is permitted to append
// media to [gearID] as a non-owner. Today that means the caller must be a
// member of at least one community the gear is shared with.
func (s *Service) callerMayAddGearMedia(
	ctx context.Context,
	gearID string,
	userID string,
) error {
	communityGears, err := s.storage.QueryByField(
		ctx, "gear_id", gearID, &models.CommunityGear{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "SaveGear",
			fmt.Errorf("failed to load shared communities: %w", err),
			"gear_id", gearID)
	}
	if len(communityGears) == 0 {
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_update", "only the owner can update this gear", nil)
	}
	communityIDs := make([]string, 0, len(communityGears))
	for _, m := range communityGears {
		communityIDs = append(communityIDs, m.(*models.CommunityGear).CommunityId)
	}
	memberships, err := s.storage.QueryByFieldIn(
		ctx, "community_id", communityIDs, &models.CommunityUser{},
	)
	if err != nil {
		return connecterr.Internal(ctx, "SaveGear",
			fmt.Errorf("failed to load community memberships: %w", err),
			"gear_id", gearID)
	}
	for _, m := range memberships {
		if m.(*models.CommunityUser).UserId == userID {
			return nil
		}
	}
	return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "media_add_requires_shared_community", "only members of a shared community can add media", nil)
}
