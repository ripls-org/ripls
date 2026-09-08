package transfer

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// listTransfers is the half of ListMyTransfers and ListReceivedTransfers that
// does not depend on which side of the transfer the viewer is on: require
// auth, apply the optional type and state filters, query, and build the API
// transfers. What genuinely differs between the two RPCs is the ownership
// field and the response type, and the response type stays with each caller.
//
// The two were 54 duplicated lines before this was extracted (#2816).
func (s *Service) listTransfers(
	ctx context.Context,
	operation string,
	roleField string,
	transferType api.TransferType,
	state api.TransferState,
) ([]*api.Transfer, error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", operation,
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)
	logger.DebugContext(ctx, "listing transfers", "role_field", roleField)

	queryFields := map[string]any{roleField: authInfo.UserID}
	if transferType != api.TransferType_TRANSFER_TYPE_UNSPECIFIED {
		queryFields["transfer_type"] = apiTransferTypeToModel(transferType)
	}
	if state != api.TransferState_TRANSFER_STATE_UNSPECIFIED {
		queryFields["state"] = apiTransferStateToModel(state)
	}

	transfersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers", "error", err)
		return nil, connecterr.Internal(ctx, operation, err)
	}

	storedTransfers := make([]*models.Transfer, 0, len(transfersProto))
	for _, msg := range transfersProto {
		storedTransfers = append(storedTransfers, msg.(*models.Transfer))
	}

	transfers, err := s.buildTransfers(ctx, storedTransfers)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build transfers", "error", err)
		return nil, connecterr.Internal(ctx, operation, err, "detail", "failed to build transfers")
	}
	return transfers, nil
}

// ListMyTransfers lists transfers made by the current user as sharer.
func (s *Service) ListMyTransfers(
	ctx context.Context,
	req *connect.Request[api.ListMyTransfersRequest],
) (*connect.Response[api.ListMyTransfersResponse], error) {
	transfers, err := s.listTransfers(ctx, "ListMyTransfers", "owner_id",
		req.Msg.TransferType, req.Msg.State)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListMyTransfersResponse{Transfers: transfers}), nil
}

// ListReceivedTransfers lists transfers received by the current user as recipient.
func (s *Service) ListReceivedTransfers(
	ctx context.Context,
	req *connect.Request[api.ListReceivedTransfersRequest],
) (*connect.Response[api.ListReceivedTransfersResponse], error) {
	transfers, err := s.listTransfers(ctx, "ListReceivedTransfers", "recipient_id",
		req.Msg.TransferType, req.Msg.State)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&api.ListReceivedTransfersResponse{Transfers: transfers}), nil
}

// GetTransfer retrieves a single transfer by ID.
func (s *Service) GetTransfer(
	ctx context.Context,
	req *connect.Request[api.GetTransferRequest],
) (*connect.Response[api.GetTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "fetching transfer",
		"transfer_id", req.Msg.TransferId,
	)

	// Fetch transfer from database
	transfer := &models.Transfer{}
	err = s.storage.GetByID(ctx, req.Msg.TransferId, transfer)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Authorization: user must be owner or recipient
	if transfer.OwnerId != authInfo.UserID && transfer.RecipientId != authInfo.UserID {
		logger.InfoContext(ctx, "user attempted to access transfer without permission",
			"transfer_id", req.Msg.TransferId,
		)
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not authorized to view this transfer"))
	}

	// Build enriched API transfer
	apiTransfer, err := s.buildTransfer(ctx, transfer)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build transfer",
			"transfer_id", req.Msg.TransferId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetTransfer", fmt.Errorf("failed to build transfer"))
	}

	return connect.NewResponse(&api.GetTransferResponse{
		Transfer: apiTransfer,
	}), nil
}

// GetGearTransfers retrieves all transfers for a gear item (including completed and cancelled).
func (s *Service) GetGearTransfers(
	ctx context.Context,
	req *connect.Request[api.GetGearTransfersRequest],
) (*connect.Response[api.GetGearTransfersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "fetching transfers for gear",
		"gear_id", req.Msg.GearId,
	)

	// Query all transfers for this gear
	transfersProto, err := s.storage.QueryByField(ctx, "gear_id", req.Msg.GearId, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers",
			"gear_id", req.Msg.GearId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetGearTransfers", err)
	}

	// Collect transfer models
	storedTransfers := make([]*models.Transfer, 0, len(transfersProto))
	for _, msg := range transfersProto {
		storedTransfers = append(storedTransfers, msg.(*models.Transfer))
	}

	// Build response with batch lookups
	transfers, err := s.buildTransfers(ctx, storedTransfers)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build transfers", "error", err)
		return nil, connecterr.Internal(ctx, "GetGearTransfers", err, "detail", "failed to build transfers")
	}

	logger.DebugContext(ctx, "found transfers for gear",
		"gear_id", req.Msg.GearId,
		"count", len(transfers),
	)

	return connect.NewResponse(&api.GetGearTransfersResponse{
		Transfers: transfers,
	}), nil
}

// GetUserTransferStatus checks the current user's transfer status for a gear item.
func (s *Service) GetUserTransferStatus(
	ctx context.Context,
	req *connect.Request[api.GetUserTransferStatusRequest],
) (*connect.Response[api.GetUserTransferStatusResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "checking transfer status for gear",
		"gear_id", req.Msg.GearId,
	)

	// Query for transfers where user is the recipient
	recipientQueryFields := map[string]any{
		"gear_id":      req.Msg.GearId,
		"recipient_id": authInfo.UserID,
	}

	recipientTransfersProto, err := s.storage.QueryByFields(ctx, recipientQueryFields, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query recipient transfers",
			"gear_id", req.Msg.GearId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "GetUserTransferStatus", err)
	}

	// Look for non-terminal transfer where user is recipient
	var activeTransfer *api.Transfer
	hasActiveTransfer := false

	for _, msg := range recipientTransfersProto {
		transfer := msg.(*models.Transfer)

		if !IsTerminalState(transfer.State) {
			apiTransfer, err := s.buildTransfer(ctx, transfer)
			if err != nil {
				logger.ErrorContext(ctx, "failed to build transfer",
					"transfer_id", transfer.Id,
					"error", err,
				)
				return nil, connecterr.Internal(ctx, "GetUserTransferStatus", err, "detail", "failed to build transfer")
			}

			activeTransfer = apiTransfer
			hasActiveTransfer = true
			logger.DebugContext(ctx, "user is recipient of active transfer",
				"transfer_id", transfer.Id,
				"gear_id", req.Msg.GearId,
			)
			break
		}
	}

	// If not found as recipient, check if user is a participant in any transfer conversation for this gear.
	if !hasActiveTransfer {
		// Query for all transfers for this gear.
		gearTransfersProto, err := s.storage.QueryByField(ctx, "gear_id", req.Msg.GearId, &models.Transfer{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query gear transfers",
				"gear_id", req.Msg.GearId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "GetUserTransferStatus", err)
		}

		// Collect non-terminal transfers for a single batch conversation fetch.
		var nonTerminalTransfers []*models.Transfer
		for _, msg := range gearTransfersProto {
			transfer := msg.(*models.Transfer)
			if !IsTerminalState(transfer.State) {
				nonTerminalTransfers = append(nonTerminalTransfers, transfer)
			}
		}

		if len(nonTerminalTransfers) > 0 {
			transferIDs := make([]string, len(nonTerminalTransfers))
			for i, t := range nonTerminalTransfers {
				transferIDs[i] = t.Id
			}

			// Batch-fetch all conversations for these transfers in one query instead of one per transfer.
			conversationsProto, err := s.storage.QueryByFieldIn(ctx, "topic_transfer_id", transferIDs, &models.ChatConversation{})
			if err != nil {
				logger.ErrorContext(ctx, "failed to query conversations for transfers",
					"gear_id", req.Msg.GearId,
					"error", err,
				)
				return nil, connecterr.Internal(ctx, "GetUserTransferStatus", err, "detail", "failed to query conversations")
			}

			// Group conversations by transfer ID.
			convsByTransfer := make(map[string][]*models.ChatConversation, len(conversationsProto))
			for _, convMsg := range conversationsProto {
				conv := convMsg.(*models.ChatConversation)
				if tid := conv.GetTopic().GetTransferId(); tid != "" {
					convsByTransfer[tid] = append(convsByTransfer[tid], conv)
				}
			}

		outer:
			for _, transfer := range nonTerminalTransfers {
				for _, conversation := range convsByTransfer[transfer.Id] {
					for _, participantID := range conversation.ParticipantIds {
						if participantID == authInfo.UserID {
							apiTransfer, err := s.buildTransfer(ctx, transfer)
							if err != nil {
								logger.ErrorContext(ctx, "failed to build transfer",
									"transfer_id", transfer.Id,
									"error", err,
								)
								return nil, connecterr.Internal(ctx, "GetUserTransferStatus", err, "detail", "failed to build transfer")
							}

							activeTransfer = apiTransfer
							hasActiveTransfer = true
							logger.DebugContext(ctx, "user is participant in conversation for active transfer",
								"transfer_id", transfer.Id,
								"gear_id", req.Msg.GearId,
							)
							break outer
						}
					}
				}
			}
		}
	}

	if !hasActiveTransfer {
		logger.DebugContext(ctx, "user has no active transfer for gear",
			"gear_id", req.Msg.GearId,
		)
	}

	return connect.NewResponse(&api.GetUserTransferStatusResponse{
		ActiveTransfer:    activeTransfer,
		HasActiveTransfer: hasActiveTransfer,
	}), nil
}

// ListTransfers lists available transfers in a community.
func (s *Service) ListTransfers(
	ctx context.Context,
	req *connect.Request[api.ListTransfersRequest],
) (*connect.Response[api.ListTransfersResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "listing transfers in community",
		"community_id", req.Msg.CommunityId,
	)

	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Build query fields
	queryFields := map[string]any{
		"community_id": req.Msg.CommunityId,
	}

	// Add optional filters
	if req.Msg.TransferType != api.TransferType_TRANSFER_TYPE_UNSPECIFIED {
		queryFields["transfer_type"] = apiTransferTypeToModel(req.Msg.TransferType)
	}
	if req.Msg.State != api.TransferState_TRANSFER_STATE_UNSPECIFIED {
		queryFields["state"] = apiTransferStateToModel(req.Msg.State)
	}

	transfersProto, err := s.storage.QueryByFields(ctx, queryFields, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query transfers",
			"community_id", req.Msg.CommunityId,
			"error", err,
		)
		return nil, connecterr.Internal(ctx, "ListTransfers", err)
	}

	// Collect transfer models
	storedTransfers := make([]*models.Transfer, 0, len(transfersProto))
	for _, msg := range transfersProto {
		storedTransfers = append(storedTransfers, msg.(*models.Transfer))
	}

	// Build response with batch lookups
	transfers, err := s.buildTransfers(ctx, storedTransfers)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build transfers", "error", err)
		return nil, connecterr.Internal(ctx, "ListTransfers", err, "detail", "failed to build transfers")
	}

	return connect.NewResponse(&api.ListTransfersResponse{
		Transfers: transfers,
	}), nil
}

// findMatchingCommunityAndType finds a community the user belongs to where the gear is shared,
// and determines the transfer type from the availability setting.
func (s *Service) findMatchingCommunityAndType(ctx context.Context, gearID, userID string) (string, models.TransferType, error) {
	logger := logging.LoggerWithContext(ctx)

	// Get all communities the gear is shared with
	sharedGears, err := s.storage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear",
			"gear_id", gearID,
			"error", err,
		)
		return "", models.TransferType_TRANSFER_TYPE_UNSPECIFIED, connecterr.Internal(ctx, "findMatchingCommunityAndType", err)
	}

	if len(sharedGears) == 0 {
		logger.InfoContext(ctx, "gear is not shared with any community",
			"gear_id", gearID,
		)
		return "", models.TransferType_TRANSFER_TYPE_UNSPECIFIED, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_not_shared_to_any_community", "this gear isn't shared with any community", nil)
	}

	// Get user's community memberships
	userMemberships, err := s.storage.QueryByField(ctx, "user_id", userID, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user memberships",
			"user_id", userID,
			"error", err,
		)
		return "", models.TransferType_TRANSFER_TYPE_UNSPECIFIED, connecterr.Internal(ctx, "findMatchingCommunityAndType", err)
	}

	// Build set of user's community IDs
	userCommunities := make(map[string]bool)
	for _, msg := range userMemberships {
		membership := msg.(*models.CommunityUser)
		userCommunities[membership.CommunityId] = true
	}

	// Find matching community and determine transfer type
	for _, msg := range sharedGears {
		communityGear := msg.(*models.CommunityGear)
		if userCommunities[communityGear.CommunityId] {
			var transferType models.TransferType
			switch communityGear.Availability {
			case models.Availability_AVAILABILITY_FOR_LOAN:
				transferType = models.TransferType_TRANSFER_TYPE_LOAN
			case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
				transferType = models.TransferType_TRANSFER_TYPE_GIVEAWAY
			default:
				logger.InfoContext(ctx, "gear availability not set",
					"gear_id", gearID,
					"community_id", communityGear.CommunityId,
					"availability", communityGear.Availability,
				)
				return "", models.TransferType_TRANSFER_TYPE_UNSPECIFIED, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_availability_not_set", "gear availability must be set", nil)
			}
			return communityGear.CommunityId, transferType, nil
		}
	}

	logger.InfoContext(ctx, "user is not a member of any community where gear is shared",
		"gear_id", gearID,
		"user_id", userID,
	)
	return "", models.TransferType_TRANSFER_TYPE_UNSPECIFIED, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("gear is not shared with any community you are a member of"))
}
