package user

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GetUserStats retrieves aggregate statistics and impact metrics for a user.
func (s *Service) GetUserStats(
	ctx context.Context,
	req *connect.Request[api.GetUserStatsRequest],
) (*connect.Response[api.GetUserStatsResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("user_id is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetUserStats",
		"user_id", req.Msg.UserId,
	)

	logger.DebugContext(ctx, "calculating user stats")

	// Verify user exists
	user := &models.User{}
	if err := s.storage.GetByID(ctx, req.Msg.UserId, user); err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("user not found: %w", err))
	}

	// Calculate all statistics
	stats, err := s.calculateUserStats(ctx, req.Msg.UserId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to calculate user stats", "error", err)
		return nil, connecterr.Internal(ctx, "GetUserStats", err)
	}

	logger.DebugContext(ctx, "user stats calculated successfully",
		"community_count", stats.CommunityCount,
		"item_count", stats.ItemCount)

	return connect.NewResponse(stats), nil
}

// calculateUserStats calculates all statistics for a user by querying storage directly.
func (s *Service) calculateUserStats(ctx context.Context, userID string) (*api.GetUserStatsResponse, error) {
	// Query community memberships
	memberships, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"user_id": userID,
	}, &models.CommunityUser{})
	if err != nil {
		return nil, fmt.Errorf("failed to query community memberships: %w", err)
	}

	// Query gear items
	gear, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"owner_id": userID,
	}, &models.Gear{})
	if err != nil {
		return nil, fmt.Errorf("failed to query gear: %w", err)
	}

	// Query transfers where user is owner (lender)
	transfersAsOwner, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"owner_id": userID,
	}, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers as owner: %w", err)
	}

	// Query transfers where user is recipient (borrower)
	transfersAsRecipient, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"recipient_id": userID,
	}, &models.Transfer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers as recipient: %w", err)
	}

	// Query non-withdrawn request offers made by this user.
	helpOffers, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"user_id":   userID,
		"withdrawn": false,
	}, &models.RequestOffer{})
	if err != nil {
		return nil, fmt.Errorf("failed to query request offers: %w", err)
	}

	// Query experiences hosted (owned) by this user.
	experiences, err := s.storage.QueryByFields(ctx, map[string]interface{}{
		"owner_id": userID,
	}, &models.Experience{})
	if err != nil {
		return nil, fmt.Errorf("failed to query experiences: %w", err)
	}

	// Count loans and giveaways
	loansCount, giveawaysCount := s.countTransfersByType(transfersAsOwner)
	borrowsCount, _ := s.countTransfersByType(transfersAsRecipient)

	// Calculate total items value
	totalItemsValue := s.calculateTotalItemsValue(gear)

	// Calculate savings from completed transfers with ImpactEstimate data
	savings := s.calculateSavings(transfersAsOwner, transfersAsRecipient)

	return &api.GetUserStatsResponse{
		CommunityCount:     int32(len(memberships)),
		ItemCount:          int32(len(gear)),
		LoansCount:         loansCount,
		BorrowsCount:       borrowsCount,
		GiveawaysCount:     giveawaysCount,
		HelpOfferedCount:   int32(len(helpOffers)),
		EventsHostedCount:  int32(len(experiences)),
		TotalItemsValueUsd: totalItemsValue,
		Savings:            savings,
	}, nil
}

// countTransfersByType counts loans and giveaways from a list of transfers.
// Returns (loans_count, giveaways_count).
func (s *Service) countTransfersByType(transfers []proto.Message) (int32, int32) {
	loansCount := int32(0)
	giveawaysCount := int32(0)

	for _, t := range transfers {
		transfer := t.(*models.Transfer)
		switch transfer.TransferType {
		case models.TransferType_TRANSFER_TYPE_LOAN:
			loansCount++
		case models.TransferType_TRANSFER_TYPE_GIVEAWAY:
			giveawaysCount++
		}
	}

	return loansCount, giveawaysCount
}

// calculateTotalItemsValue calculates the sum of estimated values for all gear items in USD.
func (s *Service) calculateTotalItemsValue(gear []proto.Message) float32 {
	totalValue := float32(0)

	for _, g := range gear {
		gearItem := g.(*models.Gear)
		if gearItem.ValueEstimate != nil {
			totalValue += gearItem.ValueEstimate.EstimatedValueUsd
		}
	}

	return totalValue
}

// calculateSavings aggregates ImpactEstimate data from completed transfers.
// It sums money saved, time saved, and emissions prevented across all
// completed transfers where the user participated as owner or recipient.
func (s *Service) calculateSavings(ownerTransfers, recipientTransfers []proto.Message) *api.UserSavings {
	var totalCostUsd float32
	var totalMinutes float32
	var totalCo2Grams float32

	// slices.Concat, not append(ownerTransfers, ...): ownerTransfers belongs to
	// the caller, and appending onto it would write into the caller's backing
	// array whenever it has spare capacity.
	all := slices.Concat(ownerTransfers, recipientTransfers)
	for _, t := range all {
		transfer := t.(*models.Transfer)
		if transfer.State != models.TransferState_TRANSFER_STATE_COMPLETED {
			continue
		}
		ie := transfer.ImpactEstimate
		if ie == nil {
			continue
		}
		if ie.MoneySaved != nil && ie.MoneySaved.ValueUsd != nil {
			totalCostUsd += ie.MoneySaved.ValueUsd.Mean
		}
		if ie.TimeSaved != nil && ie.TimeSaved.Minutes != nil {
			totalMinutes += ie.TimeSaved.Minutes.Mean
		}
		if ie.EmissionsPrevented != nil {
			if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil &&
				ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams != nil {
				totalCo2Grams += ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean
			}
			if ie.EmissionsPrevented.WasteReducedCarbon != nil &&
				ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams != nil {
				totalCo2Grams += ie.EmissionsPrevented.WasteReducedCarbon.Co2EGrams.Mean
			}
		}
	}

	return &api.UserSavings{
		CostSavedUsd:   totalCostUsd,
		TimeSavedHours: int32(totalMinutes/60 + 0.5), // Round to nearest hour.
		Co2SavedKg:     totalCo2Grams / 1000,
	}
}
