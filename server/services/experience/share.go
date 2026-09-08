package experience

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// VerifyExperienceOwner returns an error unless actorUserID owns the experience.
// Implements the audience-management owner check used by
// CommunityService.ShareItem.
func (s *Service) VerifyExperienceOwner(ctx context.Context, experienceID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx)
	expStored, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, "VerifyExperienceOwner")
	if err != nil {
		return err
	}
	if expStored.OwnerId != actorUserID {
		return connecterr.UserVisible(
			ctx,
			connect.CodePermissionDenied,
			"experience_owner_required_for_share",
			"only the owner can share this event",
			nil,
		)
	}
	return nil
}

// VerifyExperienceViewer returns the experience's owner ID after checking that
// actorUserID may view the experience — the owner, or an active member of at
// least one community it is shared with. Implements the ItemSharer view-access
// check gating link-only ShareItem calls, so any member can reshare the
// event's open link (#2630).
func (s *Service) VerifyExperienceViewer(ctx context.Context, experienceID, actorUserID string) (string, error) {
	logger := logging.LoggerWithContext(ctx)
	expStored, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, "VerifyExperienceViewer")
	if err != nil {
		return "", err
	}
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, actorUserID,
		auth.EntityExperience, expStored.Id, expStored.OwnerId,
	); err != nil {
		return "", err
	}
	return expStored.OwnerId, nil
}

// ShareExperienceToCommunity fetches the experience and shares it with the
// community idempotently. Auth (ownership + membership) is the caller's
// responsibility. Used by CommunityService.ShareItem to fan an item out to its
// ad-hoc audience and any additional communities.
func (s *Service) ShareExperienceToCommunity(ctx context.Context, experienceID, communityID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx)
	expStored, err := s.fetchExperienceForRead(ctx, experienceID, logger.Logger, "ShareExperienceToCommunity")
	if err != nil {
		return err
	}
	return s.shareExperienceToCommunity(ctx, expStored, communityID, actorUserID)
}

// shareExperienceToCommunity performs the core of sharing an experience with a
// community: ensures the experience conversation exists, auto-RSVPs the owner,
// writes the CommunityExperience junction, and emits the community event. It is
// idempotent — if the experience is already shared to the community it makes no
// changes and reports success. Auth (ownership + membership) is the caller's
// responsibility. Reached from SaveExperience (the per-item community at
// creation) and from CommunityService.ShareItem.
func (s *Service) shareExperienceToCommunity(
	ctx context.Context,
	expStored *models.Experience,
	communityID, actorUserID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "shareExperienceToCommunity",
		"experience_id", expStored.Id,
		"community_id", communityID,
	)

	// Already shared to this community? Idempotent skip.
	existing, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id":  communityID,
		"experience_id": expStored.Id,
	}, &models.CommunityExperience{})
	if err != nil {
		logger.Error("failed to check existing community experience", "error", err)
		return connecterr.Internal(ctx, "shareExperienceToCommunity", err)
	}
	if len(existing) > 0 {
		return nil
	}

	// Create or reuse the experience conversation. The conversation is now scoped to
	// the experience itself (not per-community), so use the existing one if already set.
	conversationID := expStored.ConversationId
	isNewConversation := conversationID == ""
	chatConvStorage := storage.NewChatConversationStorage(s.storage)
	if isNewConversation {
		var err2 error
		conversationID, _, err2 = chat.CreateOrGetConversation(
			ctx,
			s.storage,
			chatConvStorage,
			communityID,
			&models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: expStored.Id}},
		)
		if err2 != nil {
			logger.Error("failed to create conversation for experience", "error", err2)
			return connecterr.Internal(ctx, "shareExperienceToCommunity", err2, "detail", "failed to create conversation")
		}

		// Persist conversation_id on the experience.
		expStored.ConversationId = conversationID
		if err2 = s.storage.Update(ctx, expStored); err2 != nil {
			logger.Error("failed to persist conversation_id on experience", "error", err2)
			return connecterr.Internal(ctx, "shareExperienceToCommunity", err2, "detail", "failed to update experience")
		}
		logger.Info("created conversation for experience", "conversation_id", conversationID)
	} else {
		logger.Info("reusing existing conversation for experience", "conversation_id", conversationID)
	}

	// Add owner as initial participant to the conversation
	if err := chat.AddParticipantToConversation(ctx, chatConvStorage, conversationID, actorUserID); err != nil {
		logger.Warn("failed to add owner to conversation", "error", err)
		// Continue - conversation was created successfully
	}

	// Emit EXPERIENCE_CREATED anchor message only when the conversation is first created.
	// Subsequent shares to additional communities reuse the same conversation and must
	// not duplicate the anchor that already anchors the info pill at the top.
	if isNewConversation && s.systemMessageWriter != nil {
		displayName := s.getUserDisplayName(ctx, actorUserID)
		if err := s.systemMessageWriter.InsertLocalized(
			ctx,
			conversationID,
			actorUserID,
			models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED,
			chat.ExperienceCreatedMessage(displayName, expStored.Name),
		); err != nil {
			logger.Warn("failed to write EXPERIENCE_CREATED system message", "error", err)
			// Don't fail the share operation for system message
		}

		// Seed the creator's description as the first comment in the new
		// conversation, right after the anchor. Best-effort: a failure here must
		// not fail the share.
		if err := chat.PostCreationDescription(ctx, s.systemMessageWriter, conversationID, actorUserID, expStored.Description); err != nil {
			logger.Warn("failed to seed experience description as first comment",
				"experience_id", expStored.Id,
				"conversation_id", conversationID,
				"error", err,
			)
		}
	}

	now := clock.UnixSec(ctx)

	// Create CommunityExperience record. conversation_id is deprecated on this
	// record; the canonical conversation lives on Experience.conversation_id.
	communityExp := &models.CommunityExperience{
		CommunityId:     communityID,
		ExperienceId:    expStored.Id,
		SharedAtUnixSec: now,
	}

	if _, err := s.storage.Insert(ctx, communityExp); err != nil {
		logger.Error("failed to create CommunityExperience", "error", err)
		return connecterr.Internal(ctx, "shareExperienceToCommunity", err)
	}

	// Auto-RSVP the owner as YES if they have not already RSVPed to this
	// experience (a user may only RSVP once regardless of community count).
	// This ensures the owner's participation is always reflected in state
	// reversion checks and attendance counts.
	existingOwnerRSVPs, err := s.storage.QueryByFields(ctx, map[string]any{
		"experience_id": expStored.Id,
		"user_id":       actorUserID,
	}, &models.ExperienceRSVP{})
	if err != nil {
		logger.Warn("failed to check for existing owner RSVP", "error", err)
	} else if len(existingOwnerRSVPs) == 0 {
		ownerRSVP := &models.ExperienceRSVP{
			ExperienceId:       expStored.Id,
			UserId:             actorUserID,
			CommunityId:        communityID,
			RsvpedAtUnixSec:    now,
			LastUpdatedUnixSec: now,
		}
		ownerRSVP.Intention = models.RSVPIntention_RSVP_INTENTION_YES
		if _, err := s.storage.Insert(ctx, ownerRSVP); err != nil {
			logger.Warn("failed to create owner auto-RSVP", "error", err)
			// Don't fail the share operation for the auto-RSVP.
		}
	}

	// Record community event using the underlying Experience ID in the topic.
	event := &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:     actorUserID,
		Topic: &models.CommunityEvent_ExperienceId{
			ExperienceId: expStored.Id,
		},
		OccurredAtUnixSec: now,
	}

	if _, err := s.bus.Publish(ctx, event); err != nil {
		// Log error but don't fail the share operation
		logger.Warn("failed to record community event for experience share", "error", err)
		// Continue - the share itself succeeded
	}

	logger.Info("shared experience to community")
	return nil
}
