package community

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	chatlib "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// ListCommunityGear lists gear shared with a community.
func (s *Service) ListCommunityGear(
	ctx context.Context,
	req *connect.Request[api.ListCommunityGearRequest],
) (*connect.Response[api.ListCommunityGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	logger.DebugContext(ctx, "listing gear for community")

	// Single round-trip: fetch community + verify membership + reject deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Get all gear shared with this community (exclude archived by default)
	sharedGears, err := storage.QueryByFields[*models.CommunityGear](s.storage, ctx, map[string]any{
		"community_id": req.Msg.CommunityId,
		"archived":     false,
	})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityGear", err)
	}

	// Collect gear IDs for batch fetch.
	gearIDs := storage.CollectField(sharedGears, func(cg *models.CommunityGear) string { return cg.GearId })
	communityGearByGearID := storage.ToMap(sharedGears, func(cg *models.CommunityGear) string { return cg.GearId })

	// Batch fetch all gear (include deleted to detect soft-deleted).
	gearMap, err := storage.GetByIDs[*models.Gear](s.storage, ctx, gearIDs, storage.QueryOptions{IncludeDeleted: true})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch gear", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityGear", err, "detail",

			// Collect location IDs, owner IDs, and loan-eligible gear IDs.
			"batch fetch gear")
	}

	locationIDs := make([]string, 0)
	ownerIDs := make([]string, 0, len(gearMap))
	loanGearIDs := make([]string, 0)
	for _, gear := range gearMap {
		if gear.Deleted != nil {
			continue
		}
		ownerIDs = append(ownerIDs, gear.OwnerId)
		if gear.LocationId != "" {
			locationIDs = append(locationIDs, gear.LocationId)
		}
		cg := communityGearByGearID[gear.Id]
		if cg != nil && cg.Availability == models.Availability_AVAILABILITY_FOR_LOAN {
			loanGearIDs = append(loanGearIDs, gear.Id)
		}
	}

	// Batch fetch locations.
	locationMap, err := storage.GetByIDs[*models.Location](s.storage, ctx, locationIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch locations", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityGear", err, "detail",

			// Batch fetch owners.
			"batch fetch locations")
	}

	ownerMap, err := services.FetchAPIUsersBatch(ctx, s.storage, ownerIDs)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch owners", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityGear", err, "detail",

			// Batch fetch active loans for loan-eligible gear.
			"batch fetch owners")
	}

	activeLoanMap, err := s.batchGetActiveLoansForGear(ctx, loanGearIDs, req.Msg.CommunityId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch active loans", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityGear", err, "detail",

			// Build response items in original order.
			"batch fetch active loans")
	}

	items := make([]*api.CommunityGearItem, 0, len(sharedGears))
	for _, gearID := range gearIDs {
		gear, ok := gearMap[gearID]
		if !ok {
			continue
		}

		if gear.Deleted != nil {
			logger.WarnContext(ctx, "skipping soft-deleted gear in community listing",
				"gear_id", gearID,
				"deleted_at", gear.Deleted.DeletedAtUnixSec)
			continue
		}

		communityGear := communityGearByGearID[gearID]
		apiAvailability := modelAvailabilityToAPI(communityGear.Availability)

		var latitudeDeg, longitudeDeg float64
		if gear.LocationId != "" {
			if location, ok := locationMap[gear.LocationId]; ok {
				if location.Geolocation != nil {
					latitudeDeg = location.Geolocation.LatitudeDeg
					longitudeDeg = location.Geolocation.LongitudeDeg
				}
			}
		}

		owner := ownerMap[gear.OwnerId]

		conversationID := gear.ConversationId

		item := &api.CommunityGearItem{
			Id:             gear.Id,
			Name:           gear.Name,
			Description:    gear.Description,
			Owner:          owner,
			Availability:   apiAvailability,
			MediaIds:       gear.MediaIds,
			LocationId:     gear.LocationId,
			LatitudeDeg:    latitudeDeg,
			LongitudeDeg:   longitudeDeg,
			ConversationId: conversationID,
			ValueEstimate:  convertStorageValueEstimateToAPI(gear.ValueEstimate),
		}

		if activeLoan, ok := activeLoanMap[gearID]; ok {
			item.ActiveLoan = activeLoan
		}

		items = append(items, item)
	}

	return connect.NewResponse(&api.ListCommunityGearResponse{
		GearItems: items,
	}), nil
}

// VerifyGearOwner returns an error unless actorUserID owns the gear. Implements
// the ItemSharer owner check used by CommunityService.ShareItem.
func (s *Service) VerifyGearOwner(ctx context.Context, gearID, actorUserID string) error {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if gear.OwnerId != actorUserID {
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_share", "only the owner can share this gear", nil)
	}
	return nil
}

// VerifyGearViewer returns the gear's owner ID after checking that
// actorUserID may view the gear — the owner, or an active member of at least
// one community it is shared with. Implements the ItemSharer view-access check
// gating link-only ShareItem calls, so any member can reshare the gear's open
// link (#2630).
func (s *Service) VerifyGearViewer(ctx context.Context, gearID, actorUserID string) (string, error) {
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return "", connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}
	if _, _, err := auth.RequireAccessToCommunityScopedEntity(
		ctx, s.storage, actorUserID,
		auth.EntityGear, gear.Id, gear.OwnerId,
	); err != nil {
		return "", err
	}
	return gear.OwnerId, nil
}

// ShareGearToCommunity shares gear into a community for ShareItem (and the
// per-item-community fallback), reusing the gear's existing Lend/Give
// availability so a re-share never clobbers it; gear not yet shared anywhere
// defaults to FOR_LOAN. Auth (ownership + membership) is the caller's
// responsibility. Implements the ItemSharer ShareToCommunity hook.
func (s *Service) ShareGearToCommunity(ctx context.Context, gearID, communityID, actorUserID string) error {
	return s.shareGearToCommunity(ctx, gearID, communityID, actorUserID, s.existingGearAvailability(ctx, gearID))
}

// ShareGearToCommunityWithAvailability is ShareGearToCommunity with an
// explicit availability instead of the inherit-existing default. Used by the
// per-item community provisioner so a new gear's first share carries the
// creation-time Lend/Give choice (#2687) — every later ShareItem invite then
// inherits it via existingGearAvailability. Auth is the caller's
// responsibility.
func (s *Service) ShareGearToCommunityWithAvailability(ctx context.Context, gearID, communityID, actorUserID string, availability models.Availability) error {
	return s.shareGearToCommunity(ctx, gearID, communityID, actorUserID, availability)
}

// existingGearAvailability returns the availability the gear is already shared
// with (from any of its CommunityGear junctions), defaulting to FOR_LOAN when it
// isn't shared anywhere yet.
func (s *Service) existingGearAvailability(ctx context.Context, gearID string) models.Availability {
	rows, err := storage.QueryByField[*models.CommunityGear](s.storage, ctx, "gear_id", gearID)
	if err == nil {
		for _, cg := range rows {
			if cg.Availability == models.Availability_AVAILABILITY_FOR_LOAN ||
				cg.Availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
				return cg.Availability
			}
		}
	}
	return models.Availability_AVAILABILITY_FOR_LOAN
}

// shareGearToCommunity shares gear into a community idempotently with the given
// availability: writes (or, if already shared, updates the availability of) the
// CommunityGear junction, ensures the gear conversation + anchor system message
// on first share, and emits the GEAR_SHARED event. Auth is the caller's
// responsibility. Shared by the ShareGear RPC, ProvisionGearItemCommunity (the
// per-item community at gear creation), and the gear ItemSharer.
func (s *Service) shareGearToCommunity(ctx context.Context, gearID, communityID, actorUserID string, availability models.Availability) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "shareGearToCommunity", "gear_id", gearID, "community_id", communityID,
	)

	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}

	// Already shared? Update availability if it changed, otherwise idempotent skip.
	existingCommunityGear, err := GetCommunityGear(ctx, s.storage, communityID, gearID)
	if err != nil {
		return connecterr.Internal(ctx, "shareGearToCommunity", err)
	}
	if existingCommunityGear != nil {
		if existingCommunityGear.Availability != availability {
			existingCommunityGear.Availability = availability
			if err := s.storage.Update(ctx, existingCommunityGear); err != nil {
				return connecterr.Internal(ctx, "shareGearToCommunity", err)
			}
		}
		return nil
	}

	communityGear := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		CreatedAtUnixSec: clock.UnixSec(ctx),
		Availability:     availability,
	}
	communityGearID, err := s.storage.Insert(ctx, communityGear)
	if err != nil {
		return connecterr.Internal(ctx, "shareGearToCommunity", err)
	}

	// Create or reuse conversation for shareable gear (loan or giveaway). The
	// conversation is scoped to the gear itself (not per-community).
	if availability == models.Availability_AVAILABILITY_FOR_LOAN ||
		availability == models.Availability_AVAILABILITY_FOR_GIVEAWAY {
		conversationID := gear.ConversationId
		conversationCreated := false
		if conversationID == "" {
			conversationID, err = s.createGearConversation(ctx, gearID, communityID, actorUserID)
			if err != nil {
				return connecterr.Internal(ctx, "shareGearToCommunity", err, "detail", "failed to create gear conversation")
			}
			conversationCreated = true
			gear.ConversationId = conversationID
			if err := s.storage.Update(ctx, gear); err != nil {
				return connecterr.Internal(ctx, "shareGearToCommunity", err, "detail", "failed to update gear")
			}
		}
		communityGear.Id = communityGearID
		if err := s.storage.Update(ctx, communityGear); err != nil {
			return connecterr.Internal(ctx, "shareGearToCommunity", err, "detail", "failed to update community gear")
		}

		// Emit the sharing anchor only when the gear's conversation is first
		// created (it's per-gear, not per-community, so re-emitting on every
		// share would drop one anchor per community the gear lands in — #1679).
		if conversationCreated && s.systemMessageWriter != nil {
			displayName := s.getUserDisplayName(ctx, actorUserID)
			switch availability {
			case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
				if err := s.systemMessageWriter.InsertLocalized(ctx, conversationID, actorUserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED, chatlib.GearSharedForGiveawayMessage(displayName, gear.Name)); err != nil {
					logger.WarnContext(ctx, "failed to write GIVEAWAY_SHARED system message", "conversation_id", conversationID, "error", err)
				}
			case models.Availability_AVAILABILITY_FOR_LOAN:
				if err := s.systemMessageWriter.InsertLocalized(ctx, conversationID, actorUserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED, chatlib.GearSharedForLoanMessage(displayName, gear.Name)); err != nil {
					logger.WarnContext(ctx, "failed to write LOAN_SHARED system message", "conversation_id", conversationID, "error", err)
				}
			}

			// Seed the creator's description as the first comment, right after the
			// anchor. Best-effort: a failure here must not fail the share.
			if err := chatlib.PostCreationDescription(ctx, s.systemMessageWriter, conversationID, actorUserID, gear.Description); err != nil {
				logger.WarnContext(ctx, "failed to seed gear description as first comment",
					"gear_id", gearID, "conversation_id", conversationID, "error", err)
			}
		}
	}

	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:     actorUserID,
		GearId:      gearID,
	}); err != nil {
		return connecterr.Internal(ctx, "shareGearToCommunity", err)
	}
	return nil
}

// UpdateGearSharing updates gear sharing settings (e.g., availability).
func (s *Service) UpdateGearSharing(
	ctx context.Context,
	req *connect.Request[api.UpdateGearSharingRequest],
) (*connect.Response[api.UpdateGearSharingResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"gear_id", req.Msg.GearId,
		"community_id", req.Msg.CommunityId,
	)

	logger.InfoContext(ctx, "updating gear sharing settings")

	// Verify gear exists and user is the owner
	gear := &models.Gear{}
	err = s.storage.GetByID(ctx, req.Msg.GearId, gear)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	if gear.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_sharing_settings", "only the owner can update gear sharing settings", nil)
	}

	// Single round-trip: fetch community + verify membership + reject deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Find the share relationship
	communityGear, err := GetCommunityGear(ctx, s.storage, req.Msg.CommunityId, req.Msg.GearId)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateGearSharing", err)
	}

	if communityGear == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear is not shared with this community"))
	}

	// Update availability (convert from API type to models type)
	communityGear.Availability = models.Availability(req.Msg.Availability)

	err = s.storage.Update(ctx, communityGear)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update gear sharing", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateGearSharing", err)
	}

	return connect.NewResponse(&api.UpdateGearSharingResponse{}), nil
}

// SetGearAvailability sets a gear's availability (loan vs giveaway) across every
// community it is shared with. Unlike UpdateGearSharing (one community), this
// flips the item's mode everywhere at once, so an item is never lent in one
// community and given away in another. It is rejected while the gear has any
// in-progress transfer (interest expressed, recipient selected, or an active
// loan): changing mode mid-transfer would leave the transfer's type and the
// gear's availability contradicting each other. On success it updates every
// CommunityGear junction and writes the loan/giveaway anchor system message once
// into the gear's (single, per-gear) conversation.
//
// It deliberately emits no CommunityEvent: GEAR_SHARED drives a feed card and a
// community-wide "shared an item" push, which would be spurious for a re-mode of
// an already-shared item. Cache freshness for the actor is handled by the client
// repository; the conversation anchor is the durable record for participants.
func (s *Service) SetGearAvailability(
	ctx context.Context,
	req *connect.Request[api.SetGearAvailabilityRequest],
) (*connect.Response[api.SetGearAvailabilityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"gear_id", req.Msg.GearId,
	)

	availability := apiAvailabilityToModel(req.Msg.Availability)
	if availability != models.Availability_AVAILABILITY_FOR_LOAN &&
		availability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
		return nil, connecterr.UserVisible(ctx, connect.CodeInvalidArgument, "gear_availability_invalid", "availability must be loan or giveaway", nil)
	}

	// Verify gear exists and the caller owns it.
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, req.Msg.GearId, gear); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if gear.OwnerId != authInfo.UserID {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_availability", "only the owner can change how this item is shared", nil)
	}

	// Guard: refuse while any transfer for this gear is still in progress.
	// A change of mode mid-transfer would leave the transfer's type and the
	// gear's availability contradicting each other (a giveaway skips the ACTIVE
	// loan state entirely). Completed/cancelled transfers are terminal and do
	// not block.
	transfers, err := storage.QueryByField[*models.Transfer](s.storage, ctx, "gear_id", req.Msg.GearId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "SetGearAvailability", err)
	}
	for _, t := range transfers {
		switch t.State {
		case models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
			models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
			models.TransferState_TRANSFER_STATE_ACTIVE:
			logger.WarnContext(ctx, "rejecting gear availability change: gear has an in-progress transfer",
				"transfer_id", t.Id, "transfer_state", t.State.String())
			return nil, connecterr.UserVisible(ctx, connect.CodeFailedPrecondition, "gear_availability_change_blocked_active_transfer", "finish or cancel the in-progress loan or giveaway before changing how this item is shared", nil)
		}
	}

	// Enumerate every community this gear is shared with and flip any whose
	// availability differs from the target.
	junctions, err := storage.QueryByField[*models.CommunityGear](s.storage, ctx, "gear_id", req.Msg.GearId)
	if err != nil {
		return nil, connecterr.Internal(ctx, "SetGearAvailability", err)
	}

	changedCommunities := make([]string, 0, len(junctions))
	for _, cg := range junctions {
		if cg.Availability == availability {
			continue
		}
		cg.Availability = availability
		if err := s.storage.Update(ctx, cg); err != nil {
			return nil, connecterr.Internal(ctx, "SetGearAvailability", err)
		}
		changedCommunities = append(changedCommunities, cg.CommunityId)
	}

	if len(changedCommunities) == 0 {
		// Already in the requested mode everywhere — idempotent no-op.
		logger.InfoContext(ctx, "gear availability already set; nothing to change",
			"availability", availability.String(), "communities_total", len(junctions))
		return connect.NewResponse(&api.SetGearAvailabilityResponse{}), nil
	}

	// Announce the change once in the gear's conversation (it's per-gear, not
	// per-community, so one message covers every community). Best-effort.
	s.announceGearAvailabilityChange(ctx, gear, authInfo.UserID, availability)

	logger.InfoContext(ctx, "set gear availability across communities",
		"availability", availability.String(),
		"communities_changed", len(changedCommunities),
		"communities_total", len(junctions))

	return connect.NewResponse(&api.SetGearAvailabilityResponse{}), nil
}

// announceGearAvailabilityChange writes the loan/giveaway anchor system message
// into the gear's conversation when its availability changes. Best-effort: a
// failure to announce must not fail the availability change itself.
func (s *Service) announceGearAvailabilityChange(ctx context.Context, gear *models.Gear, actorUserID string, availability models.Availability) {
	if s.systemMessageWriter == nil || gear.ConversationId == "" {
		return
	}
	logger := logging.LoggerWithContext(ctx)
	displayName := s.getUserDisplayName(ctx, actorUserID)
	switch availability {
	case models.Availability_AVAILABILITY_FOR_GIVEAWAY:
		if err := s.systemMessageWriter.InsertLocalized(ctx, gear.ConversationId, actorUserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED, chatlib.GearSharedForGiveawayMessage(displayName, gear.Name)); err != nil {
			logger.WarnContext(ctx, "failed to write GIVEAWAY_SHARED system message on availability change", "conversation_id", gear.ConversationId, "error", err)
		}
	case models.Availability_AVAILABILITY_FOR_LOAN:
		if err := s.systemMessageWriter.InsertLocalized(ctx, gear.ConversationId, actorUserID, models.ChatSystemAction_CHAT_SYSTEM_ACTION_LOAN_SHARED, chatlib.GearSharedForLoanMessage(displayName, gear.Name)); err != nil {
			logger.WarnContext(ctx, "failed to write LOAN_SHARED system message on availability change", "conversation_id", gear.ConversationId, "error", err)
		}
	}
}

// UnshareGear unshares gear from a community.
// UnshareGearFromCommunity removes gear from a community by deleting the
// CommunityGear junction and emitting a GEAR_UNSHARED event. Owner-only;
// idempotent when the gear isn't shared with the community. Implements the
// ItemSharer UnshareFromCommunity hook used by CommunityService.UnshareItem.
func (s *Service) UnshareGearFromCommunity(ctx context.Context, gearID, communityID, actorUserID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", actorUserID,
		"gear_id", gearID,
		"community_id", communityID,
	)

	logger.InfoContext(ctx, "unsharing gear from community")

	// Verify gear exists and user is the owner
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, gearID, gear); err != nil {
		logger.ErrorContext(ctx, "failed to get gear", "error", err)
		return connect.NewError(connect.CodeNotFound, err)
	}

	if gear.OwnerId != actorUserID {
		return connecterr.UserVisible(ctx, connect.CodePermissionDenied, "gear_owner_required_for_unshare", "only the owner can unshare this gear", nil)
	}

	// Find the share relationship
	communityGear, err := GetCommunityGear(ctx, s.storage, communityID, gearID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query community gear", "error", err)
		return connecterr.Internal(ctx, "UnshareGear", err)
	}

	if communityGear == nil {
		// Not shared - idempotent
		return nil
	}

	// Delete the share relationship
	if err := s.storage.Delete(ctx, communityGear); err != nil {
		logger.ErrorContext(ctx, "failed to unshare gear", "error", err)
		return connecterr.Internal(ctx, "UnshareGear", err)
	}

	// Record event
	if _, err := s.bus.Publish(ctx, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED,
		ActorId:     actorUserID,
		GearId:      gearID,
	}); err != nil {
		return connecterr.Internal(ctx, "UnshareGear", err)
	}

	return nil
}

// IsGearSharedWithCommunity checks if a piece of gear is shared with a community.
// Returns true if the gear is shared, false otherwise.
func IsGearSharedWithCommunity(ctx context.Context, s *storage.ProtoSQLStorage, communityID, gearID string) (bool, error) {
	messages, err := s.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"gear_id":      gearID,
	}, &models.CommunityGear{})
	if err != nil {
		return false, fmt.Errorf("failed to query gear sharing: %w", err)
	}

	return len(messages) > 0, nil
}

// GetCommunityGear retrieves a community gear relationship record.
// Returns the record if found, nil otherwise.
func GetCommunityGear(ctx context.Context, s *storage.ProtoSQLStorage, communityID, gearID string) (*models.CommunityGear, error) {
	results, err := storage.QueryByFields[*models.CommunityGear](s, ctx, map[string]any{
		"community_id": communityID,
		"gear_id":      gearID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query gear sharing: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// createGearConversation creates a conversation for a gear item in a community.
// Uses the shared chat library to avoid service dependencies.
func (s *Service) createGearConversation(ctx context.Context, gearID, communityID, ownerID string) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "createGearConversation",
		"gear_id", gearID,
		"community_id", communityID,
		"owner_id", ownerID,
	)

	logger.DebugContext(ctx, "creating conversation for gear")

	// Create conversation using shared chat library
	topic := &models.ConversationTopic{
		TopicId: &models.ConversationTopic_GearId{GearId: gearID},
	}

	chatConvStorage := storage.NewChatConversationStorage(s.storage)
	conversationID, participantIDs, err := chatlib.CreateOrGetConversation(ctx, s.storage, chatConvStorage, communityID, topic)
	if err != nil {
		return "", fmt.Errorf("failed to create conversation: %w", err)
	}

	// Add owner as initial participant
	alreadyParticipant := false
	for _, pid := range participantIDs {
		if pid == ownerID {
			alreadyParticipant = true
			break
		}
	}

	if !alreadyParticipant {
		if err := chatlib.AddParticipantToConversation(ctx, chatConvStorage, conversationID, ownerID); err != nil {
			logger.ErrorContext(ctx, "failed to add owner to gear conversation", "conversation_id", conversationID, "error", err)
			return "", fmt.Errorf("failed to add owner to gear conversation: %w", err)
		}
	}

	logger.InfoContext(ctx, "created conversation for gear", "conversation_id", conversationID)
	return conversationID, nil
}

// batchGetActiveLoansForGear fetches active loans for multiple gear IDs in a
// community using batch queries, returning a map of gearID → *api.ActiveLoan.
func (s *Service) batchGetActiveLoansForGear(ctx context.Context, gearIDs []string, communityID string) (map[string]*api.ActiveLoan, error) {
	result := make(map[string]*api.ActiveLoan)
	if len(gearIDs) == 0 {
		return result, nil
	}

	// Fetch all active loan transfers for the given gear IDs in this community.
	allTransfers, err := storage.QueryByFieldIn[*models.Transfer](s.storage, ctx, "gear_id", gearIDs)
	if err != nil {
		return nil, fmt.Errorf("batch query active loans: %w", err)
	}

	// Filter to active loans in this community and collect borrower IDs.
	type loanInfo struct {
		transfer *models.Transfer
	}
	loansByGear := make(map[string]*loanInfo)
	borrowerIDs := make([]string, 0)
	for _, t := range allTransfers {
		if t.CommunityId != communityID ||
			t.TransferType != models.TransferType_TRANSFER_TYPE_LOAN ||
			t.State != models.TransferState_TRANSFER_STATE_ACTIVE {
			continue
		}
		// Take first active loan per gear (should only be one).
		if _, exists := loansByGear[t.GearId]; !exists {
			loansByGear[t.GearId] = &loanInfo{transfer: t}
			borrowerIDs = append(borrowerIDs, t.RecipientId)
		}
	}

	if len(loansByGear) == 0 {
		return result, nil
	}

	// Batch fetch borrower users.
	borrowerMap, err := services.FetchAPIUsersBatch(ctx, s.storage, borrowerIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch borrowers: %w", err)
	}

	for gearID, info := range loansByGear {
		borrower := borrowerMap[info.transfer.RecipientId]
		result[gearID] = &api.ActiveLoan{
			Borrower:   borrower,
			TransferId: info.transfer.Id,
			Status:     "On loan",
		}
	}

	return result, nil
}

// convertStorageValueEstimateToAPI converts a storage model ValueEstimate to an API ValueEstimate.
func convertStorageValueEstimateToAPI(storageEstimate *models.ValueEstimate) *api.ValueEstimate {
	if storageEstimate == nil {
		return nil
	}
	result := &api.ValueEstimate{
		EstimatedValueUsd: storageEstimate.EstimatedValueUsd,
	}
	if storageEstimate.Provenance != nil {
		result.Provenance = &api.Provenance{
			Source:     api.ProvenanceSource(storageEstimate.Provenance.Source),
			Name:       storageEstimate.Provenance.Name,
			Version:    storageEstimate.Provenance.Version,
			Confidence: storageEstimate.Provenance.Confidence,
			Reasoning:  storageEstimate.Provenance.Reasoning,
			Sources:    storageEstimate.Provenance.Sources,
		}
	}
	return result
}
