package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// UnshareExperienceFromCommunity removes an experience from a community by
// deleting the CommunityExperience record. Owner-only; rejects a soft-deleted
// target community and returns NotFound when the experience isn't shared there.
// Implements the ItemSharer UnshareFromCommunity hook used by
// CommunityService.UnshareItem.
func (s *Service) UnshareExperienceFromCommunity(ctx context.Context, experienceID, communityID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", actorUserID,
		"experience_id", experienceID,
		"community_id", communityID,
	)

	expStored, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, "UnshareExperience")
	if err != nil {
		return err
	}

	// Verify ownership
	if expStored.OwnerId != actorUserID {
		return connecterr.UserVisible(
			ctx,
			connect.CodePermissionDenied,
			"experience_owner_required_for_unshare",
			"only the owner can unshare this event",
			nil,
		)
	}

	// Reject if the target community has been soft-deleted.
	if _, err := auth.RequireActiveCommunity(ctx, s.storage, communityID); err != nil {
		return err
	}

	// Find CommunityExperience record
	queryFields := map[string]any{
		"community_id":  communityID,
		"experience_id": experienceID,
	}
	communityExps, err := s.storage.QueryByFields(ctx, queryFields, &models.CommunityExperience{})
	if err != nil {
		logger.Error("failed to find community experience", "error", err)
		return connecterr.Internal(ctx, "UnshareExperience", err)
	}
	if len(communityExps) == 0 {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not shared to this community"))
	}

	communityExp := communityExps[0].(*models.CommunityExperience)

	// Delete CommunityExperience record
	if err := s.storage.Delete(ctx, communityExp); err != nil {
		logger.Error("failed to delete community experience", "error", err)
		return connecterr.Internal(ctx, "UnshareExperience", err)
	}

	logger.Info("unshared experience from community")
	return nil
}
