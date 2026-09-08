package storage

import (
	"context"
	"errors"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// isNotFoundError checks if an error indicates a record was not found.
func isNotFoundError(err error) bool {
	return errors.Is(err, ErrRecordNotFound)
}

// CascadeDeleteConversationsByTopic soft-deletes all conversations linked to an entity by topic field.
// The topicField should be one of: "gear_id", "experience_id", "request_id".
// Conversations that are already deleted are skipped gracefully.
func CascadeDeleteConversationsByTopic(ctx context.Context, storage *ProtoSQLStorage, topicField, topicID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteConversationsByTopic",
		topicField, topicID,
	)

	conversations, err := storage.QueryByField(ctx, topicField, topicID, &models.ChatConversation{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query conversations for cascade deletion", "error", err)
		return fmt.Errorf("failed to query conversations for deletion: %w", err)
	}

	for _, convProto := range conversations {
		conversation := convProto.(*models.ChatConversation)
		if conversation.Deleted != nil && conversation.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted conversation", "conversation_id", conversation.Id)
			continue
		}
		conversation.Deleted = deletedMetadata
		if err := storage.Update(ctx, conversation); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete conversation", "conversation_id", conversation.Id, "error", err)
			return fmt.Errorf("failed to delete conversation: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted conversation", "conversation_id", conversation.Id)
	}

	return nil
}

// CascadeDeleteConversationByID soft-deletes a conversation by its ID.
// If the conversation doesn't exist or is already deleted, no error is returned.
func CascadeDeleteConversationByID(ctx context.Context, storage *ProtoSQLStorage, conversationID string, deletedMetadata *models.DeletedMetadata) error {
	if conversationID == "" {
		return nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteConversationByID",
		"conversation_id", conversationID,
	)

	conversation := &models.ChatConversation{}
	if err := storage.GetByID(ctx, conversationID, conversation, QueryOptions{IncludeDeleted: true}); err != nil {
		if isNotFoundError(err) {
			logger.DebugContext(ctx, "skipping non-existent conversation in cascade delete")
			return nil
		}
		logger.ErrorContext(ctx, "failed to get conversation for cascade deletion", "error", err)
		return fmt.Errorf("failed to get conversation for deletion: %w", err)
	}

	if conversation.Deleted != nil && conversation.Deleted.DeletedAtUnixSec > 0 {
		logger.DebugContext(ctx, "skipping already deleted conversation")
		return nil
	}

	conversation.Deleted = deletedMetadata
	if err := storage.Update(ctx, conversation); err != nil {
		logger.ErrorContext(ctx, "failed to cascade delete conversation", "error", err)
		return fmt.Errorf("failed to delete conversation: %w", err)
	}
	logger.DebugContext(ctx, "cascade deleted conversation")
	return nil
}

// CascadeDeleteCommunityGearByGearID soft-deletes all community_gear join rows
// for the given gear, including rows already marked archived. Called from
// DeleteGear so downstream readers stop surfacing the dead gear_id. Uses
// IncludeDeleted: true on the initial query so already-soft-deleted rows are
// skipped idempotently rather than invisible.
func CascadeDeleteCommunityGearByGearID(ctx context.Context, storage *ProtoSQLStorage, gearID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityGearByGearID",
		"gear_id", gearID,
	)

	rows, err := storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_gear for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_gear for deletion: %w", err)
	}

	for _, m := range rows {
		cg := m.(*models.CommunityGear)
		if cg.Deleted != nil && cg.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_gear", "community_gear_id", cg.Id)
			continue
		}
		cg.Deleted = deletedMetadata
		if err := storage.Update(ctx, cg); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_gear", "community_gear_id", cg.Id, "error", err)
			return fmt.Errorf("failed to delete community_gear: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_gear", "community_gear_id", cg.Id, "community_id", cg.CommunityId)
	}

	return nil
}

// CascadeDeleteCommunityRequestByRequestID soft-deletes all community_request
// join rows for the given request. See CascadeDeleteCommunityGearByGearID.
func CascadeDeleteCommunityRequestByRequestID(ctx context.Context, storage *ProtoSQLStorage, requestID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityRequestByRequestID",
		"request_id", requestID,
	)

	rows, err := storage.QueryByField(ctx, "request_id", requestID, &models.CommunityRequest{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_request for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_request for deletion: %w", err)
	}

	for _, m := range rows {
		cr := m.(*models.CommunityRequest)
		if cr.Deleted != nil && cr.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_request", "community_request_id", cr.Id)
			continue
		}
		cr.Deleted = deletedMetadata
		if err := storage.Update(ctx, cr); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_request", "community_request_id", cr.Id, "error", err)
			return fmt.Errorf("failed to delete community_request: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_request", "community_request_id", cr.Id, "community_id", cr.CommunityId)
	}

	return nil
}

// CascadeDeleteCommunityExperienceByExperienceID soft-deletes all
// community_experience join rows for the given experience. See
// CascadeDeleteCommunityGearByGearID.
func CascadeDeleteCommunityExperienceByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityExperienceByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.CommunityExperience{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_experience for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_experience for deletion: %w", err)
	}

	for _, m := range rows {
		ce := m.(*models.CommunityExperience)
		if ce.Deleted != nil && ce.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_experience", "community_experience_id", ce.Id)
			continue
		}
		ce.Deleted = deletedMetadata
		if err := storage.Update(ctx, ce); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_experience", "community_experience_id", ce.Id, "error", err)
			return fmt.Errorf("failed to delete community_experience: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_experience", "community_experience_id", ce.Id, "community_id", ce.CommunityId)
	}

	return nil
}

// CascadeDeletePlanningNeedsByExperienceID soft-deletes all planning_need
// rows scoped to the given experience. The experience_id field is part of
// the scope oneof but protosql flattens it to a queryable column — see
// server/planning/list.go for an existing read-side caller.
func CascadeDeletePlanningNeedsByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeletePlanningNeedsByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.PlanningNeed{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query planning_need for cascade deletion", "error", err)
		return fmt.Errorf("failed to query planning_need for deletion: %w", err)
	}

	for _, m := range rows {
		need := m.(*models.PlanningNeed)
		if need.Deleted != nil && need.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted planning_need", "planning_need_id", need.Id)
			continue
		}
		need.Deleted = deletedMetadata
		if err := storage.Update(ctx, need); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete planning_need", "planning_need_id", need.Id, "error", err)
			return fmt.Errorf("failed to delete planning_need: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted planning_need", "planning_need_id", need.Id)
	}

	return nil
}

// CascadeDeletePlanningContributionsByExperienceID soft-deletes all
// planning_contribution rows scoped to the given experience.
func CascadeDeletePlanningContributionsByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeletePlanningContributionsByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.PlanningContribution{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query planning_contribution for cascade deletion", "error", err)
		return fmt.Errorf("failed to query planning_contribution for deletion: %w", err)
	}

	for _, m := range rows {
		contrib := m.(*models.PlanningContribution)
		if contrib.Deleted != nil && contrib.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted planning_contribution", "planning_contribution_id", contrib.Id)
			continue
		}
		contrib.Deleted = deletedMetadata
		if err := storage.Update(ctx, contrib); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete planning_contribution", "planning_contribution_id", contrib.Id, "error", err)
			return fmt.Errorf("failed to delete planning_contribution: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted planning_contribution", "planning_contribution_id", contrib.Id)
	}

	return nil
}

// CascadeDeleteExperienceRSVPsByExperienceID soft-deletes all
// experience_rsvp rows for the given experience. RSVPs are
// participant commitments; once the experience is gone, they should
// stop surfacing on "my upcoming events" / RSVP screens for non-owner
// participants.
func CascadeDeleteExperienceRSVPsByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteExperienceRSVPsByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query experience_rsvp for cascade deletion", "error", err)
		return fmt.Errorf("failed to query experience_rsvp for deletion: %w", err)
	}

	for _, m := range rows {
		rsvp := m.(*models.ExperienceRSVP)
		if rsvp.Deleted != nil && rsvp.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted experience_rsvp", "experience_rsvp_id", rsvp.Id)
			continue
		}
		rsvp.Deleted = deletedMetadata
		if err := storage.Update(ctx, rsvp); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete experience_rsvp", "experience_rsvp_id", rsvp.Id, "error", err)
			return fmt.Errorf("failed to delete experience_rsvp: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted experience_rsvp", "experience_rsvp_id", rsvp.Id, "user_id", rsvp.UserId)
	}

	return nil
}

// CascadeDeleteExperienceTimeProposalsByExperienceID soft-deletes all
// experience_time_proposal rows for the given experience.
func CascadeDeleteExperienceTimeProposalsByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteExperienceTimeProposalsByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceTimeProposal{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query experience_time_proposal for cascade deletion", "error", err)
		return fmt.Errorf("failed to query experience_time_proposal for deletion: %w", err)
	}

	for _, m := range rows {
		prop := m.(*models.ExperienceTimeProposal)
		if prop.Deleted != nil && prop.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted experience_time_proposal", "experience_time_proposal_id", prop.Id)
			continue
		}
		prop.Deleted = deletedMetadata
		if err := storage.Update(ctx, prop); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete experience_time_proposal", "experience_time_proposal_id", prop.Id, "error", err)
			return fmt.Errorf("failed to delete experience_time_proposal: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted experience_time_proposal", "experience_time_proposal_id", prop.Id)
	}

	return nil
}

// CascadeDeleteExperienceLocationProposalsByExperienceID soft-deletes all
// experience_location_proposal rows for the given experience.
func CascadeDeleteExperienceLocationProposalsByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteExperienceLocationProposalsByExperienceID",
		"experience_id", experienceID,
	)

	rows, err := storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceLocationProposal{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query experience_location_proposal for cascade deletion", "error", err)
		return fmt.Errorf("failed to query experience_location_proposal for deletion: %w", err)
	}

	for _, m := range rows {
		prop := m.(*models.ExperienceLocationProposal)
		if prop.Deleted != nil && prop.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted experience_location_proposal", "experience_location_proposal_id", prop.Id)
			continue
		}
		prop.Deleted = deletedMetadata
		if err := storage.Update(ctx, prop); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete experience_location_proposal", "experience_location_proposal_id", prop.Id, "error", err)
			return fmt.Errorf("failed to delete experience_location_proposal: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted experience_location_proposal", "experience_location_proposal_id", prop.Id)
	}

	return nil
}

// CascadeDeleteCommunityGearByCommunityID soft-deletes every
// community_gear join row scoped to the given community. Mirrors
// CascadeDeleteCommunityGearByGearID but filters on community_id —
// used by DeleteCommunity to detach all shared gear in one pass.
// Idempotent: rows already marked deleted are skipped.
func CascadeDeleteCommunityGearByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityGearByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityGear{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_gear for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_gear for deletion: %w", err)
	}

	for _, m := range rows {
		cg := m.(*models.CommunityGear)
		if cg.Deleted != nil && cg.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_gear", "community_gear_id", cg.Id)
			continue
		}
		cg.Deleted = deletedMetadata
		if err := storage.Update(ctx, cg); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_gear", "community_gear_id", cg.Id, "error", err)
			return fmt.Errorf("failed to delete community_gear: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_gear", "community_gear_id", cg.Id, "gear_id", cg.GearId)
	}

	return nil
}

// CascadeDeleteCommunityRequestByCommunityID soft-deletes every
// community_request join row scoped to the given community. See
// CascadeDeleteCommunityGearByCommunityID.
func CascadeDeleteCommunityRequestByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityRequestByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_request for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_request for deletion: %w", err)
	}

	for _, m := range rows {
		cr := m.(*models.CommunityRequest)
		if cr.Deleted != nil && cr.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_request", "community_request_id", cr.Id)
			continue
		}
		cr.Deleted = deletedMetadata
		if err := storage.Update(ctx, cr); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_request", "community_request_id", cr.Id, "error", err)
			return fmt.Errorf("failed to delete community_request: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_request", "community_request_id", cr.Id, "request_id", cr.RequestId)
	}

	return nil
}

// CascadeDeleteCommunityExperienceByCommunityID soft-deletes every
// community_experience join row scoped to the given community. See
// CascadeDeleteCommunityGearByCommunityID.
func CascadeDeleteCommunityExperienceByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityExperienceByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_experience for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_experience for deletion: %w", err)
	}

	for _, m := range rows {
		ce := m.(*models.CommunityExperience)
		if ce.Deleted != nil && ce.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_experience", "community_experience_id", ce.Id)
			continue
		}
		ce.Deleted = deletedMetadata
		if err := storage.Update(ctx, ce); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_experience", "community_experience_id", ce.Id, "error", err)
			return fmt.Errorf("failed to delete community_experience: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_experience", "community_experience_id", ce.Id, "experience_id", ce.ExperienceId)
	}

	return nil
}

// CascadeDeleteCommunityNotificationPreferencesByCommunityID soft-deletes
// every community_notification_preferences row scoped to the given community.
// See CascadeDeleteCommunityGearByCommunityID.
func CascadeDeleteCommunityNotificationPreferencesByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityNotificationPreferencesByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityNotificationPreferences{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_notification_preferences for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_notification_preferences for deletion: %w", err)
	}

	for _, m := range rows {
		prefs := m.(*models.CommunityNotificationPreferences)
		if prefs.Deleted != nil && prefs.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_notification_preferences", "row_id", prefs.Id)
			continue
		}
		prefs.Deleted = deletedMetadata
		if err := storage.Update(ctx, prefs); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_notification_preferences", "row_id", prefs.Id, "error", err)
			return fmt.Errorf("failed to delete community_notification_preferences: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_notification_preferences", "row_id", prefs.Id, "user_id", prefs.UserId)
	}

	return nil
}

// CascadeDeleteCommunityInvitationLinkByCommunityID soft-deletes every
// community_invitation_link row scoped to the given community. The
// is_revoked flag stays untouched — that's the user-facing manual
// revoke; deletion vs revoke are distinct states.
//
// TODO(#2056): Remove this function and its call sites once the legacy
// community_invitation_link table has been dropped. ShareLink rows are
// cascaded by CascadeDeleteShareLinksByCommunityID below.
func CascadeDeleteCommunityInvitationLinkByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteCommunityInvitationLinkByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityInvitationLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_invitation_link for cascade deletion", "error", err)
		return fmt.Errorf("failed to query community_invitation_link for deletion: %w", err)
	}

	for _, m := range rows {
		link := m.(*models.CommunityInvitationLink)
		if link.Deleted != nil && link.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted community_invitation_link", "link_id", link.Id)
			continue
		}
		link.Deleted = deletedMetadata
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete community_invitation_link", "link_id", link.Id, "error", err)
			return fmt.Errorf("failed to delete community_invitation_link: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted community_invitation_link", "link_id", link.Id, "short_code", link.ShortCode)
	}

	return nil
}

// CascadeDeleteShareLinksByCommunityID soft-deletes every share_link row
// scoped to the given community, regardless of which oneof target variant
// is populated. The is_revoked flag stays untouched.
func CascadeDeleteShareLinksByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeDeleteShareLinksByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.ShareLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query share_link for cascade deletion", "error", err)
		return fmt.Errorf("failed to query share_link for deletion: %w", err)
	}

	for _, m := range rows {
		link := m.(*models.ShareLink)
		if link.Deleted != nil && link.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted share_link", "share_link_id", link.Id)
			continue
		}
		link.Deleted = deletedMetadata
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete share_link", "share_link_id", link.Id, "error", err)
			return fmt.Errorf("failed to delete share_link: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted share_link",
			"share_link_id", link.Id,
			"short_code", logging.MaskToken(link.ShortCode),
		)
	}

	return nil
}

// cascadeDeleteShareLinksByTargetField is the shared body for
// CascadeDeleteShareLinks{ByExperienceID,ByGearID,…} target-side helpers.
// Soft-deletes every share_link row where the named target column equals
// the given ID — used when the target entity (an experience, a gear, a
// transfer, a request) is itself soft-deleted, so the share links that
// point at it stop working.
func cascadeDeleteShareLinksByTargetField(ctx context.Context, storage *ProtoSQLStorage, op, targetField, targetID string, deletedMetadata *models.DeletedMetadata) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", op,
		targetField, targetID,
	)

	rows, err := storage.QueryByField(ctx, targetField, targetID, &models.ShareLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query share_link for cascade deletion", "error", err)
		return fmt.Errorf("failed to query share_link for deletion: %w", err)
	}

	for _, m := range rows {
		link := m.(*models.ShareLink)
		if link.Deleted != nil && link.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted share_link", "share_link_id", link.Id)
			continue
		}
		link.Deleted = deletedMetadata
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete share_link", "share_link_id", link.Id, "error", err)
			return fmt.Errorf("failed to delete share_link: %w", err)
		}
		logger.DebugContext(ctx, "cascade deleted share_link",
			"share_link_id", link.Id,
			"short_code", logging.MaskToken(link.ShortCode),
		)
	}

	return nil
}

// CascadeDeleteShareLinksByExperienceID soft-deletes share_link rows
// whose experience_id matches.
func CascadeDeleteShareLinksByExperienceID(ctx context.Context, storage *ProtoSQLStorage, experienceID string, deletedMetadata *models.DeletedMetadata) error {
	return cascadeDeleteShareLinksByTargetField(ctx, storage, "CascadeDeleteShareLinksByExperienceID", "experience_id", experienceID, deletedMetadata)
}

// CascadeDeleteShareLinksByGearID soft-deletes share_link rows whose
// gear_id matches.
func CascadeDeleteShareLinksByGearID(ctx context.Context, storage *ProtoSQLStorage, gearID string, deletedMetadata *models.DeletedMetadata) error {
	return cascadeDeleteShareLinksByTargetField(ctx, storage, "CascadeDeleteShareLinksByGearID", "gear_id", gearID, deletedMetadata)
}

// CascadeDeleteShareLinksByTransferID soft-deletes share_link rows whose
// transfer_id matches.
func CascadeDeleteShareLinksByTransferID(ctx context.Context, storage *ProtoSQLStorage, transferID string, deletedMetadata *models.DeletedMetadata) error {
	return cascadeDeleteShareLinksByTargetField(ctx, storage, "CascadeDeleteShareLinksByTransferID", "transfer_id", transferID, deletedMetadata)
}

// CascadeDeleteShareLinksByRequestID soft-deletes share_link rows whose
// request_id matches.
func CascadeDeleteShareLinksByRequestID(ctx context.Context, storage *ProtoSQLStorage, requestID string, deletedMetadata *models.DeletedMetadata) error {
	return cascadeDeleteShareLinksByTargetField(ctx, storage, "CascadeDeleteShareLinksByRequestID", "request_id", requestID, deletedMetadata)
}

// wasDeletedByCommunityCascade reports whether a join row's
// soft-delete metadata matches the community's own delete cascade —
// i.e. same actor, and the row was deleted at or after the
// community itself. Used by the CascadeRestore* helpers to honor
// the §5 rule: "Restore reverses the Delete, not history. Anything
// that was already cancelled or removed *before* the delete stays
// cancelled.".
func wasDeletedByCommunityCascade(rowDeleted *models.DeletedMetadata, communityDeletedBy string, communityDeletedAt int64) bool {
	if rowDeleted == nil || rowDeleted.DeletedAtUnixSec == 0 {
		return false
	}
	if rowDeleted.DeletedByUserId != communityDeletedBy {
		return false
	}
	return rowDeleted.DeletedAtUnixSec >= communityDeletedAt
}

// CascadeRestoreCommunityGearByCommunityID clears the soft-delete
// marker on every community_gear join row that was soft-deleted as
// part of the community's own delete cascade. Rows that were
// soft-deleted earlier (e.g. by a prior leave or by deleting the
// underlying gear) stay soft-deleted, per §5.
func CascadeRestoreCommunityGearByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreCommunityGearByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityGear{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_gear for cascade restore", "error", err)
		return fmt.Errorf("failed to query community_gear for restore: %w", err)
	}

	for _, m := range rows {
		cg := m.(*models.CommunityGear)
		if !wasDeletedByCommunityCascade(cg.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		cg.Deleted = nil
		if err := storage.Update(ctx, cg); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore community_gear", "community_gear_id", cg.Id, "error", err)
			return fmt.Errorf("failed to restore community_gear: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored community_gear", "community_gear_id", cg.Id, "gear_id", cg.GearId)
	}

	return nil
}

// CascadeRestoreCommunityRequestByCommunityID is the inverse of
// CascadeDeleteCommunityRequestByCommunityID, scoped to rows the
// community-delete cascade soft-deleted. See
// CascadeRestoreCommunityGearByCommunityID.
func CascadeRestoreCommunityRequestByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreCommunityRequestByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityRequest{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_request for cascade restore", "error", err)
		return fmt.Errorf("failed to query community_request for restore: %w", err)
	}

	for _, m := range rows {
		cr := m.(*models.CommunityRequest)
		if !wasDeletedByCommunityCascade(cr.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		cr.Deleted = nil
		if err := storage.Update(ctx, cr); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore community_request", "community_request_id", cr.Id, "error", err)
			return fmt.Errorf("failed to restore community_request: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored community_request", "community_request_id", cr.Id, "request_id", cr.RequestId)
	}

	return nil
}

// CascadeRestoreCommunityExperienceByCommunityID is the inverse of
// CascadeDeleteCommunityExperienceByCommunityID. See
// CascadeRestoreCommunityGearByCommunityID.
func CascadeRestoreCommunityExperienceByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreCommunityExperienceByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityExperience{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_experience for cascade restore", "error", err)
		return fmt.Errorf("failed to query community_experience for restore: %w", err)
	}

	for _, m := range rows {
		ce := m.(*models.CommunityExperience)
		if !wasDeletedByCommunityCascade(ce.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		ce.Deleted = nil
		if err := storage.Update(ctx, ce); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore community_experience", "community_experience_id", ce.Id, "error", err)
			return fmt.Errorf("failed to restore community_experience: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored community_experience", "community_experience_id", ce.Id, "experience_id", ce.ExperienceId)
	}

	return nil
}

// CascadeRestoreCommunityNotificationPreferencesByCommunityID is the
// inverse of CascadeDeleteCommunityNotificationPreferencesByCommunityID.
// See CascadeRestoreCommunityGearByCommunityID.
func CascadeRestoreCommunityNotificationPreferencesByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreCommunityNotificationPreferencesByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityNotificationPreferences{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_notification_preferences for cascade restore", "error", err)
		return fmt.Errorf("failed to query community_notification_preferences for restore: %w", err)
	}

	for _, m := range rows {
		prefs := m.(*models.CommunityNotificationPreferences)
		if !wasDeletedByCommunityCascade(prefs.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		prefs.Deleted = nil
		if err := storage.Update(ctx, prefs); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore community_notification_preferences", "row_id", prefs.Id, "error", err)
			return fmt.Errorf("failed to restore community_notification_preferences: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored community_notification_preferences", "row_id", prefs.Id, "user_id", prefs.UserId)
	}

	return nil
}

// CascadeRestoreCommunityInvitationLinkByCommunityID is the inverse
// of CascadeDeleteCommunityInvitationLinkByCommunityID. See
// CascadeRestoreCommunityGearByCommunityID. Does not touch
// is_revoked — links manually revoked stay revoked.
//
// TODO(#2056): Remove this function and its call sites once the legacy
// community_invitation_link table has been dropped. ShareLink rows are
// restored by CascadeRestoreShareLinksByCommunityID below.
func CascadeRestoreCommunityInvitationLinkByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreCommunityInvitationLinkByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.CommunityInvitationLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community_invitation_link for cascade restore", "error", err)
		return fmt.Errorf("failed to query community_invitation_link for restore: %w", err)
	}

	for _, m := range rows {
		link := m.(*models.CommunityInvitationLink)
		if !wasDeletedByCommunityCascade(link.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		link.Deleted = nil
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore community_invitation_link", "link_id", link.Id, "error", err)
			return fmt.Errorf("failed to restore community_invitation_link: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored community_invitation_link", "link_id", link.Id, "short_code", link.ShortCode)
	}

	return nil
}

// CascadeRestoreShareLinksByCommunityID is the inverse of
// CascadeDeleteShareLinksByCommunityID. Does not touch is_revoked —
// links manually revoked stay revoked.
func CascadeRestoreShareLinksByCommunityID(ctx context.Context, storage *ProtoSQLStorage, communityID, communityDeletedBy string, communityDeletedAt int64) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CascadeRestoreShareLinksByCommunityID",
		"community_id", communityID,
	)

	rows, err := storage.QueryByField(ctx, "community_id", communityID, &models.ShareLink{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query share_link for cascade restore", "error", err)
		return fmt.Errorf("failed to query share_link for restore: %w", err)
	}

	for _, m := range rows {
		link := m.(*models.ShareLink)
		if !wasDeletedByCommunityCascade(link.Deleted, communityDeletedBy, communityDeletedAt) {
			continue
		}
		link.Deleted = nil
		if err := storage.Update(ctx, link); err != nil {
			logger.ErrorContext(ctx, "failed to cascade restore share_link", "share_link_id", link.Id, "error", err)
			return fmt.Errorf("failed to restore share_link: %w", err)
		}
		logger.DebugContext(ctx, "cascade restored share_link",
			"share_link_id", link.Id,
			"short_code", logging.MaskToken(link.ShortCode),
		)
	}

	return nil
}

// CascadeDeleteMedia soft-deletes all media items with the given IDs.
// Media that is already deleted or doesn't exist is skipped gracefully.
// The same DeletedMetadata is applied to all media items.
func CascadeDeleteMedia(ctx context.Context, storage *ProtoSQLStorage, mediaIDs []string, deletedMetadata *models.DeletedMetadata) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	logger := logging.LoggerWithContext(ctx).With("operation", "CascadeDeleteMedia")

	for _, mediaID := range mediaIDs {
		media := &models.Media{}
		err := storage.GetByID(ctx, mediaID, media, QueryOptions{IncludeDeleted: true})
		if err != nil {
			if isNotFoundError(err) {
				logger.DebugContext(ctx, "skipping non-existent media in cascade delete", "media_id", mediaID)
				continue
			}
			logger.ErrorContext(ctx, "failed to get media for cascade delete", "media_id", mediaID, "error", err)
			return err
		}

		// Skip already deleted media
		if media.Deleted != nil && media.Deleted.DeletedAtUnixSec > 0 {
			logger.DebugContext(ctx, "skipping already deleted media", "media_id", mediaID)
			continue
		}

		media.Deleted = deletedMetadata
		if err := storage.Update(ctx, media); err != nil {
			logger.ErrorContext(ctx, "failed to cascade delete media", "media_id", mediaID, "error", err)
			return err
		}
		logger.DebugContext(ctx, "cascade deleted media", "media_id", mediaID)
	}

	return nil
}
