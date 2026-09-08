package gear

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// GetGearPeople retrieves people associated with a gear item (owner, borrowers, interested parties).
func (s *Service) GetGearPeople(
	ctx context.Context,
	req *connect.Request[api.GetGearPeopleRequest],
) (*connect.Response[api.GetGearPeopleResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetGearPeople",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"gear_id", req.Msg.GearId,
	)

	logger.DebugContext(ctx, "fetching gear people")

	// Fetch gear including soft-deleted rows so transfer participants can
	// still see who else was involved after the gear is deleted (#1701).
	gear := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.GearId, gear, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.ErrorContext(ctx, "failed to get gear", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connecterr.Internal(ctx, "GetGearPeople", err)
	}

	gearDeleted := gear.Deleted != nil
	if gearDeleted {
		logger.WarnContext(
			ctx, "gear is soft-deleted; serving people from surviving Transfer rows",
			"reason", "soft_deleted",
			"deleted_at_unix_sec", gear.Deleted.DeletedAtUnixSec,
		)
	}

	// Live gear: standard access check (owner, or member of a shared community).
	// Soft-deleted gear: cascade has removed community_gear rows, so the
	// standard check would always deny non-owners with dangling transfers —
	// exactly the case this branch exists to serve. Skip the check; the
	// response surfaces only display data the caller previously had access to.
	if !gearDeleted {
		if _, _, err := auth.RequireReadAccessToCommunityScopedEntity(
			ctx, s.storage, authInfo.UserID,
			auth.EntityGear, gear.Id, gear.OwnerId,
		); err != nil {
			return nil, err
		}
	}

	// Fetch owner details. Deleted owners surface as a former-member
	// placeholder so the gear's people view remains usable.
	owner, err := services.FetchAPIUser(ctx, s.storage, gear.OwnerId)
	if err != nil {
		if errors.Is(err, storage.ErrRecordNotFound) {
			logger.WarnContext(
				ctx, "gear owner is no longer available; using former-member placeholder",
				"owner_id", gear.OwnerId,
			)
			owner = services.FormerMemberPlaceholder(gear.OwnerId)
		} else {
			logger.ErrorContext(
				ctx, "failed to fetch gear owner",
				"owner_id", gear.OwnerId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "GetGearPeople", fmt.Errorf("failed to fetch owner"))
		}
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
		logger.ErrorContext(
			ctx, "failed to query transfers",
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetGearPeople", fmt.Errorf("failed to query transfers"))
	}

	// Collect unique recipient IDs for batch fetch
	recipientIDSet := make(map[string]bool)
	for _, msg := range transfersProto {
		transfer := msg.(*models.Transfer)
		if transfer.RecipientId != "" && transfer.TransferType != models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			recipientIDSet[transfer.RecipientId] = true
		}
	}
	recipientIDs := make([]string, 0, len(recipientIDSet))
	for id := range recipientIDSet {
		recipientIDs = append(recipientIDs, id)
	}

	// Batch fetch all recipient users
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, recipientIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch recipient users", "error", err)
		return nil, connecterr.Internal(ctx, "GetGearPeople", fmt.Errorf("failed to fetch recipient users"))
	}

	var currentBorrower *api.User
	var pastBorrowers []*api.User

	// Process transfers to extract borrowers
	for _, msg := range transfersProto {
		transfer := msg.(*models.Transfer)

		// Skip if no recipient selected
		if transfer.RecipientId == "" {
			continue
		}

		// Skip giveaways (these are permanent transfers, not loans)
		if transfer.TransferType == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
			continue
		}

		// Deleted recipients surface as a former-member placeholder so the
		// gear's borrow history stays visible.
		recipientUser := services.ResolveUserOrFormer(userMap, transfer.RecipientId)

		// Determine if this is the current borrower or a past borrower
		isActive := transfer.State == models.TransferState_TRANSFER_STATE_ACTIVE
		if isActive && currentBorrower == nil {
			currentBorrower = recipientUser
		} else if transfer.State == models.TransferState_TRANSFER_STATE_COMPLETED {
			// Add to past borrowers if not already added and not the current borrower
			alreadyAdded := false
			for _, pb := range pastBorrowers {
				if pb.Id == transfer.RecipientId {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded && (currentBorrower == nil || currentBorrower.Id != transfer.RecipientId) {
				pastBorrowers = append(pastBorrowers, recipientUser)
			}
		}
	}

	// For interested parties, we could query chat participants or view history
	// For now, leaving this empty as the spec doesn't define how to track interest
	var interestedParties []*api.User

	// Calculate total count (owner + current borrower + past borrowers + interested)
	totalCount := int32(1) // Always count the owner
	if currentBorrower != nil {
		totalCount++
	}
	totalCount += int32(len(pastBorrowers))
	totalCount += int32(len(interestedParties))

	logger.DebugContext(
		ctx, "fetched gear people",
		"total_count", totalCount,
		"current_borrower", currentBorrower != nil,
		"past_borrowers_count", len(pastBorrowers),
	)

	return connect.NewResponse(&api.GetGearPeopleResponse{
		Owner:             owner,
		CurrentBorrower:   currentBorrower,
		PastBorrowers:     pastBorrowers,
		InterestedParties: interestedParties,
		TotalCount:        totalCount,
	}), nil
}
