package story_subscriber

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/story"
)

// handleTransferCompleted generates a story when a transfer (loan or
// giveaway) completes. Moved verbatim from
// services/transfer/lifecycle.go::generateStoryForCompletedTransfer.
func (s *Subscriber) handleTransferCompleted(ctx context.Context, evt *cebus.PublishedEvent) {
	transfer := evt.Transfer
	if transfer == nil {
		return
	}
	gear := evt.Gear
	if gear == nil {
		gear = &models.Gear{}
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "story_subscriber.transfer",
		"transfer_id", transfer.Id,
		"transfer_type", transfer.TransferType,
	)

	// Fetch the owner.
	userMap, err := storage.GetByIDs[*models.User](s.storage, ctx, []string{transfer.OwnerId})
	if err != nil {
		logger.WarnContext(ctx, "failed to fetch owner for transfer story", "error", err)
		return
	}
	owner, ok := userMap[transfer.OwnerId]
	if !ok {
		logger.WarnContext(ctx, "owner not found for transfer story", "owner_id", transfer.OwnerId)
		return
	}

	// Resolve recipient name and ID — may be a registered user or a provisional user.
	recipientName := ""
	recipientID := transfer.RecipientId
	if transfer.RecipientId != "" {
		recpMap, recpErr := storage.GetByIDs[*models.User](s.storage, ctx, []string{transfer.RecipientId})
		if recpErr != nil {
			logger.WarnContext(ctx, "failed to fetch recipient for transfer story", "error", recpErr)
			return
		}
		recp, found := recpMap[transfer.RecipientId]
		if !found {
			logger.WarnContext(ctx, "recipient not found for transfer story", "recipient_id", transfer.RecipientId)
			return
		}
		recipientName = recp.Name
	} else if transfer.ProvisionalRecipientId != nil && *transfer.ProvisionalRecipientId != "" {
		prov := &models.ProvisionalUser{}
		if provisionalErr := s.storage.GetByID(ctx, *transfer.ProvisionalRecipientId, prov); provisionalErr != nil {
			logger.WarnContext(ctx, "failed to fetch provisional recipient for transfer story", "error", provisionalErr)
			return
		}
		recipientName = prov.Name
		// Provisional users have no registered ID; use an empty string so the
		// story records the owner as the sole participant with full impact
		// credit.
		recipientID = ""
	}

	// Determine story type based on transfer type.
	var storyType, trigger string
	switch transfer.TransferType {
	case models.TransferType_TRANSFER_TYPE_LOAN:
		storyType = ai.StoryTypeLoanCompleted
		trigger = "loan_return"
	case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
		storyType = ai.StoryTypeGiveawayCompleted
		trigger = "giveaway_complete"
	default:
		logger.WarnContext(ctx, "unsupported transfer type for story", "transfer_type", transfer.TransferType)
		return
	}
	logger.InfoContext(ctx, "story generation triggered",
		"story_type", storyType,
		"trigger", trigger,
	)

	// Build participant lists in the correct order based on transfer type.
	var participantIDs []string
	var participantNames []string
	if transfer.TransferType == models.TransferType_TRANSFER_TYPE_LOAN {
		// For loans: borrower first, then lender.
		participantIDs = []string{recipientID, transfer.OwnerId}
		participantNames = []string{recipientName, owner.Name}
	} else {
		// For giveaways: giver first, then receiver.
		participantIDs = []string{transfer.OwnerId, recipientID}
		participantNames = []string{owner.Name, recipientName}
	}
	// Drop empty IDs (provisional users) from the participant list.
	filteredIDs := participantIDs[:0]
	filteredNames := participantNames[:0]
	for i, id := range participantIDs {
		if id != "" {
			filteredIDs = append(filteredIDs, id)
			filteredNames = append(filteredNames, participantNames[i])
		}
	}
	participantIDs = filteredIDs
	participantNames = filteredNames

	ie := impact_metrics.ModelsImpactToAPI(transfer.ImpactEstimate)
	storyReq := story.CreateStoryRequest{
		CommunityID:       transfer.CommunityId,
		StoryType:         storyType,
		ParticipantIDs:    participantIDs,
		ParticipantNames:  participantNames,
		MediaIDs:          gear.MediaIds,
		RelatedEntityID:   transfer.Id,
		RelatedEntityName: gear.Name,
		RelatedGearID:     transfer.GearId,
		RelatedLoanID:     transfer.Id, // For loans, the transfer IS the loan
		CostSavedUSD:      impact_metrics.CostUSD(ie),
		TimeSavedMinutes:  impact_metrics.TimeMinutes(ie),
		Co2SavedGrams:     impact_metrics.Co2Grams(ie),
		CommunityEventID:  evt.Event.Id,
	}

	storyStart := time.Now()
	if _, err := ai.CallWithTimeout(ctx, 60*time.Second, func(ctx context.Context) (*models.Story, error) {
		return s.creator.CreateStory(ctx, storyReq)
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.WarnContext(ctx, "LLM call exceeded timeout budget",
				"external_service", "ai_provider",
				"operation", "CreateStory",
				"duration_ms", time.Since(storyStart).Milliseconds(),
				"error", err)
		} else {
			logger.WarnContext(ctx, "failed to create story for transfer", "error", fmt.Errorf("create story: %w", err))
		}
		return
	}
	logger.InfoContext(ctx, "story created for completed transfer")
}
