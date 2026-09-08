package community

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// ListCompletedGiveaways lists gear that was given away in a community.
// Returns archived CommunityGear records where the giveaway was completed.
func (s *Service) ListCompletedGiveaways(
	ctx context.Context,
	req *connect.Request[api.ListCompletedGiveawaysRequest],
) (*connect.Response[api.ListCompletedGiveawaysResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListCompletedGiveaways",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	logger.DebugContext(ctx, "listing past giveaways for community")

	// Single round-trip: fetch community + verify membership + reject deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Get all archived gear in this community (past giveaways)
	archivedGears, err := s.storage.QueryByFields(ctx, map[string]any{
		"community_id": req.Msg.CommunityId,
		"archived":     true,
	}, &models.CommunityGear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query past giveaways", "error", err)
		return nil, connecterr.Internal(ctx, "ListCompletedGiveaways", err)
	}

	logger.InfoContext(ctx, "retrieved past giveaways", "count", len(archivedGears))

	// Fetch full gear details for each archived gear
	items := make([]*api.CommunityGearItem, 0, len(archivedGears))
	for _, msg := range archivedGears {
		communityGear := msg.(*models.CommunityGear)

		// Get the gear details
		gear := &models.Gear{}
		err = s.storage.GetByID(ctx, communityGear.GearId, gear)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get gear", "gear_id", communityGear.GearId, "error", err)
			return nil, connecterr.Internal(ctx, "ListCompletedGiveaways", err, "detail", "failed to get gear %s")
		}

		// Get location details if available
		var latitudeDeg, longitudeDeg float64
		if gear.LocationId != "" {
			location := &models.Location{}
			err = s.storage.GetByID(ctx, gear.LocationId, location)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get location for gear",
					"location_id", gear.LocationId,
					"gear_id", gear.Id,
					"error", err)
				return nil, connecterr.Internal(ctx, "ListCompletedGiveaways", err, "detail", "failed to get location %s for gear %s")
			}
			if location.Geolocation != nil {
				latitudeDeg = location.Geolocation.LatitudeDeg
				longitudeDeg = location.Geolocation.LongitudeDeg
			}
		}

		// Get owner details (original owner who gave the item away)
		owner, err := services.FetchAPIUser(ctx, s.storage, gear.OwnerId)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get owner for gear",
				"owner_id", gear.OwnerId,
				"gear_id", gear.Id,
				"error", err)
			return nil, connecterr.Internal(ctx, "ListCompletedGiveaways", err)
		}

		conversationID := gear.ConversationId

		// Build the gear item
		item := &api.CommunityGearItem{
			Id:             gear.Id,
			Name:           gear.Name,
			Description:    gear.Description,
			Owner:          owner,
			Availability:   api.Availability_AVAILABILITY_FOR_GIVEAWAY, // All past giveaways were for giveaway
			MediaIds:       gear.MediaIds,
			LocationId:     gear.LocationId,
			LatitudeDeg:    latitudeDeg,
			LongitudeDeg:   longitudeDeg,
			ConversationId: conversationID,
		}

		items = append(items, item)
	}

	return connect.NewResponse(&api.ListCompletedGiveawaysResponse{
		GearItems: items,
	}), nil
}
