package experience

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// DeleteExperience soft-deletes an experience from the database.
// The associated conversation (if any) is also soft-deleted.
func (s *Service) DeleteExperience(
	ctx context.Context,
	req *connect.Request[api.DeleteExperienceRequest],
) (*connect.Response[api.DeleteExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteExperience",
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	logger.Info("deleting experience")

	expStored, err := s.fetchExperienceForRead(ctx, req.Msg.ExperienceId, logger.Logger, "DeleteExperience")
	if err != nil {
		return nil, err
	}

	// Verify ownership
	if expStored.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(
			ctx,
			connect.CodePermissionDenied,
			"experience_owner_required_for_delete",
			"only the owner can delete this event",
			nil,
		)
	}

	// Set the deleted metadata (soft delete)
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: clock.UnixSec(ctx),
	}
	expStored.Deleted = deletedMetadata

	err = s.storage.Update(ctx, expStored)
	if err != nil {
		logger.Error("failed to soft-delete experience", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}

	// Cascade deletion to all conversations linked to this experience
	if err := storage.CascadeDeleteConversationsByTopic(ctx, s.storage, "topic_experience_id", req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}

	// Cascade deletion to community_experience join rows so the deleted
	// experience stops surfacing in community feeds and past-event listings.
	if err := storage.CascadeDeleteCommunityExperienceByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}

	// Cascade deletion to participant commitment rows: RSVPs, time
	// proposals, and planning needs/contributions scoped to this
	// experience. Activity-log rows (CommunityEvent, Story) are
	// intentionally left intact — see #1698 for the shape rationale.
	if err := storage.CascadeDeleteExperienceRSVPsByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}
	if err := storage.CascadeDeleteExperienceTimeProposalsByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}
	if err := storage.CascadeDeleteExperienceLocationProposalsByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}
	if err := storage.CascadeDeletePlanningNeedsByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}
	if err := storage.CascadeDeletePlanningContributionsByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}

	// Cascade deletion to share links that point at this experience.
	if err := storage.CascadeDeleteShareLinksByExperienceID(ctx, s.storage, req.Msg.ExperienceId, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err)
	}

	// Cascade deletion to associated media
	if err := storage.CascadeDeleteMedia(ctx, s.storage, expStored.MediaIds, deletedMetadata); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteExperience", err, "detail", "failed to cascade delete media")
	}

	logger.Info("experience deleted successfully")

	return connect.NewResponse(&api.DeleteExperienceResponse{}), nil
}
