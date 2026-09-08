package chat

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// appendMediaToParentEntity appends chat media IDs to the parent entity's media list.
// This allows chat images to appear in the main carousel for the item.
func (s *Service) appendMediaToParentEntity(
	ctx context.Context,
	conversation *models.ChatConversation,
	mediaIDs []string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "appendMediaToParentEntity",
		"conversation_id", conversation.Id,
		"media_count", len(mediaIDs),
	)

	// Log conversation topic for debugging
	convTopic := conversation.GetTopic()
	if convTopic == nil {
		logger.DebugContext(ctx, "conversation topic is nil - direct message conversation")
		return nil
	}

	logger.DebugContext(ctx, "conversation has topic",
		"topic_type", fmt.Sprintf("%T", convTopic.GetTopicId()),
	)

	switch topic := convTopic.GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		return s.appendMediaToTransferGear(ctx, topic.TransferId, mediaIDs, logger)

	case *models.ConversationTopic_GearId:
		// Community gear conversation - append directly to gear
		return s.appendMediaToGear(ctx, topic.GearId, mediaIDs, logger)

	case *models.ConversationTopic_RequestId:
		return s.appendMediaToRequest(ctx, topic.RequestId, mediaIDs, logger)

	case *models.ConversationTopic_ExperienceId:
		return s.appendMediaToExperience(ctx, topic.ExperienceId, mediaIDs, logger)

	case *models.ConversationTopic_CommunityId:
		return s.appendMediaToCommunity(ctx, topic.CommunityId, mediaIDs, logger)

	default:
		// Direct message conversations have no parent entity
		logger.DebugContext(ctx, "conversation has no parent entity, skipping media append")
		return nil
	}
}

// appendMediaToTransferGear appends media to the gear associated with a transfer.
func (s *Service) appendMediaToTransferGear(
	ctx context.Context,
	transferID string,
	mediaIDs []string,
	logger *logging.Logger,
) error {
	logger = logger.With("transfer_id", transferID)

	// 1. Get transfer to find the gear ID
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transfer); err != nil {
		logger.ErrorContext(ctx, "transfer not found", "error", err)
		return fmt.Errorf("transfer not found: %w", err)
	}

	logger = logger.With("gear_id", transfer.GearId)

	// 2. Get gear
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
		logger.ErrorContext(ctx, "gear not found", "error", err)
		return fmt.Errorf("gear not found: %w", err)
	}

	// 3. Append media IDs to gear (avoid duplicates)
	originalCount := len(gear.MediaIds)
	gear.MediaIds = appendUniqueMediaIds(gear.MediaIds, mediaIDs)
	newCount := len(gear.MediaIds) - originalCount

	// 4. Update gear
	if err := s.storage.Update(ctx, gear); err != nil {
		logger.ErrorContext(ctx, "failed to update gear media", "error", err)
		return fmt.Errorf("failed to update gear: %w", err)
	}

	logger.InfoContext(ctx, "media appended to gear",
		"added_count", newCount,
		"total_media", len(gear.MediaIds),
	)

	return nil
}

// appendMediaToGear appends media directly to a gear (for community gear conversations).
func (s *Service) appendMediaToGear(
	ctx context.Context,
	gearID string,
	mediaIDs []string,
	logger *logging.Logger,
) error {
	logger = logger.With("gear_id", gearID)

	// 1. Get gear
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.ErrorContext(ctx, "gear not found", "error", err)
		return fmt.Errorf("gear not found: %w", err)
	}

	// 2. Append media IDs to gear (avoid duplicates)
	originalCount := len(gear.MediaIds)
	gear.MediaIds = appendUniqueMediaIds(gear.MediaIds, mediaIDs)
	newCount := len(gear.MediaIds) - originalCount

	// 3. Update gear
	if err := s.storage.Update(ctx, gear); err != nil {
		logger.ErrorContext(ctx, "failed to update gear media", "error", err)
		return fmt.Errorf("failed to update gear: %w", err)
	}

	logger.InfoContext(ctx, "media appended to gear",
		"added_count", newCount,
		"total_media", len(gear.MediaIds),
	)

	return nil
}

// appendMediaToExperience appends media to an experience.
func (s *Service) appendMediaToExperience(
	ctx context.Context,
	experienceID string,
	mediaIDs []string,
	logger *logging.Logger,
) error {
	logger = logger.With("experience_id", experienceID)

	// 1. Get experience
	experience := &models.Experience{}
	if err := s.storage.GetByID(ctx, experienceID, experience); err != nil {
		logger.ErrorContext(ctx, "experience not found", "error", err)
		return fmt.Errorf("experience not found: %w", err)
	}

	// 2. Append media IDs (avoid duplicates)
	originalCount := len(experience.MediaIds)
	experience.MediaIds = appendUniqueMediaIds(experience.MediaIds, mediaIDs)
	newCount := len(experience.MediaIds) - originalCount

	// 3. Update experience
	if err := s.storage.Update(ctx, experience); err != nil {
		logger.ErrorContext(ctx, "failed to update experience media", "error", err)
		return fmt.Errorf("failed to update experience: %w", err)
	}

	logger.InfoContext(ctx, "media appended to experience",
		"added_count", newCount,
		"total_media", len(experience.MediaIds),
	)

	return nil
}

// appendMediaToCommunity appends media to a community's media list.
// Mirrors appendMediaToGear/Experience/Request: chat-attached images sent
// in a community-wide conversation surface in the community's main media
// carousel, so any member's contribution becomes part of the shared gallery.
func (s *Service) appendMediaToCommunity(
	ctx context.Context,
	communityID string,
	mediaIDs []string,
	logger *logging.Logger,
) error {
	logger = logger.With("community_id", communityID)

	community := &models.Community{}
	if err := s.storage.GetByID(ctx, communityID, community); err != nil {
		logger.ErrorContext(ctx, "community not found", "error", err)
		return fmt.Errorf("community not found: %w", err)
	}

	originalCount := len(community.MediaIds)
	community.MediaIds = appendUniqueMediaIds(community.MediaIds, mediaIDs)
	newCount := len(community.MediaIds) - originalCount

	if err := s.storage.Update(ctx, community); err != nil {
		logger.ErrorContext(ctx, "failed to update community media", "error", err)
		return fmt.Errorf("failed to update community: %w", err)
	}

	logger.InfoContext(ctx, "media appended to community",
		"added_count", newCount,
		"total_media", len(community.MediaIds),
	)

	return nil
}

// appendMediaToRequest appends media to a request.
func (s *Service) appendMediaToRequest(
	ctx context.Context,
	requestID string,
	mediaIDs []string,
	logger *logging.Logger,
) error {
	logger = logger.With("target_request_id", requestID)

	// 1. Get request
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		logger.ErrorContext(ctx, "request not found", "error", err)
		return fmt.Errorf("request not found: %w", err)
	}

	// 2. Append media IDs (avoid duplicates)
	originalCount := len(request.MediaIds)
	request.MediaIds = appendUniqueMediaIds(request.MediaIds, mediaIDs)
	newCount := len(request.MediaIds) - originalCount

	// 3. Update request
	if err := s.storage.Update(ctx, request); err != nil {
		logger.ErrorContext(ctx, "failed to update request media", "error", err)
		return fmt.Errorf("failed to update request: %w", err)
	}

	logger.InfoContext(ctx, "media appended to request",
		"added_count", newCount,
		"total_media", len(request.MediaIds),
	)

	return nil
}

// appendUniqueMediaIds appends new media IDs to existing list, skipping duplicates.
func appendUniqueMediaIds(existing, updated []string) []string {
	// Build set of existing IDs for O(1) lookup
	existingSet := make(map[string]bool, len(existing))
	for _, id := range existing {
		existingSet[id] = true
	}

	// Append only updated IDs
	result := make([]string, len(existing), len(existing)+len(updated))
	copy(result, existing)

	for _, id := range updated {
		if !existingSet[id] {
			result = append(result, id)
			existingSet[id] = true // Prevent duplicates within updated list
		}
	}

	return result
}
