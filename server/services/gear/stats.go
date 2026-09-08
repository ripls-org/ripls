package gear

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/logging"
)

// countTimesLoaned returns the number of completed and active (non-giveaway) loan
// transfers for a gear item. Used to guard the "View Impact" menu item.
func (s *Service) countTimesLoaned(ctx context.Context, gearID string) (int32, error) {
	transfersProto, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		return 0, fmt.Errorf("failed to query transfers: %w", err)
	}
	var count int32
	for _, msg := range transfersProto {
		transfer := msg.(*models.Transfer)
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			continue
		}
		if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED ||
			transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE {
			count++
		}
	}
	return count, nil
}

// GetGearStats retrieves statistics for a gear item (loans, people helped, value shared).
func (s *Service) GetGearStats(
	ctx context.Context,
	req *connect.Request[api.GetGearStatsRequest],
) (*connect.Response[api.GetGearStatsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"gear_id", req.Msg.GearId,
	)

	logger.DebugContext(ctx, "fetching gear stats")

	// Fetch gear to verify it exists and get estimated value
	gear := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.GearId, gear)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get gear",
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	// Caller must be an active member of at least one community the gear is
	// (or was) shared with, or be the owner — read gate, see #2695.
	if _, _, err := auth.RequireReadAccessToCommunityScopedEntity(
		ctx, s.storage, authInfo.UserID,
		auth.EntityGear, gear.Id, gear.OwnerId,
	); err != nil {
		return nil, err
	}

	// Get estimated value in USD (default to 0 if not set)
	estimatedValueUSD := float32(0)
	if gear.ValueEstimate != nil && gear.ValueEstimate.EstimatedValueUsd > 0 {
		estimatedValueUSD = gear.ValueEstimate.EstimatedValueUsd
	}

	// Build query for transfers related to this gear
	queryFields := map[string]any{
		"gear_id": req.Msg.GearId,
	}

	// If community context is provided, filter by community. Reject the
	// request if the supplied community is missing or soft-deleted.
	if req.Msg.CommunityId != "" {
		if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
			return nil, err
		}
		queryFields["community_id"] = req.Msg.CommunityId
	}

	// Fetch all transfers for this gear
	transfersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetGearStats", fmt.Errorf("failed to query transfers"))
	}

	// Calculate statistics from transfers
	var timesLoaned int32
	uniqueBorrowers := make(map[string]struct{})
	var interestCount int32

	for _, msg := range transfersProto {
		transfer := msg.(*models.Transfer)

		// Skip giveaways (these are permanent transfers, not loans)
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			continue
		}

		// Count completed loans
		if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED {
			timesLoaned++
			if transfer.RecipientId != "" {
				uniqueBorrowers[transfer.RecipientId] = struct{}{}
			}
		}

		// Count active loans
		if transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE {
			timesLoaned++
			if transfer.RecipientId != "" {
				uniqueBorrowers[transfer.RecipientId] = struct{}{}
			}
		}

		// Count interest expressions (people who showed interest)
		if transfer.State == models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED ||
			transfer.State == models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
			interestCount++
		}
	}

	// Calculate people helped (unique borrowers)
	peopleHelped := int32(len(uniqueBorrowers))

	// Calculate value shared (estimated value × times loaned)
	valueSharedUSD := estimatedValueUSD * float32(timesLoaned)

	// Calculate cumulative impact across all loans (only when loans exist).
	var totalImpact *api.ImpactEstimate
	if timesLoaned > 0 && s.estimatorCfg != nil {
		totalImpact = impact_metrics.BuildGearCumulativeImpactMetrics(gear, timesLoaned, s.estimatorCfg)
	}

	// Calculate per-loan impact (always populated). Used by SharingImpactCard to show
	// what each individual loan saves — independent of how many loans have occurred.
	var potentialImpact *api.ImpactEstimate
	if s.estimatorCfg != nil {
		potentialImpact = impact_metrics.BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, s.estimatorCfg, nil, nil)
	}

	logger.DebugContext(ctx, "calculated gear stats",
		"times_loaned", timesLoaned,
		"people_helped", peopleHelped,
		"value_shared_usd", valueSharedUSD,
		"interest_count", interestCount,
		"has_impact", totalImpact != nil,
		"has_potential_impact", potentialImpact != nil,
	)

	return connect.NewResponse(&api.GetGearStatsResponse{
		TimesLoaned:     timesLoaned,
		PeopleHelped:    peopleHelped,
		ValueSharedUsd:  valueSharedUSD,
		InterestCount:   interestCount,
		Impact:          totalImpact,
		PotentialImpact: potentialImpact,
	}), nil
}
