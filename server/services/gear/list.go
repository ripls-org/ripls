package gear

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
)

// ListUserGear retrieves the authenticated user's gear from the database.
func (s *Service) ListUserGear(
	ctx context.Context, req *connect.Request[api.ListUserGearRequest],
) (*connect.Response[api.ListUserGearResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With("user_id", authInfo.UserID)
	logger.Debug("listing user gear")

	// Query gear by owner_id at the database level for efficiency
	messages, err := s.storage.QueryByField(ctx, "owner_id", authInfo.UserID, &models.Gear{})
	if err != nil {
		logger.Error("failed to list user gear", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserGear", err)
	}

	// Fetch owner details once since all gear belongs to the same user
	owner, err := services.FetchAPIUser(ctx, s.storage, authInfo.UserID)
	if err != nil {
		logger.Error("failed to get owner for user gear", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserGear", fmt.Errorf("failed to load owner information"))
	}

	// Build a map of gear_id -> shared community IDs
	sharedCommunityMap, err := s.getSharedCommunityMap(ctx, messages)
	if err != nil {
		logger.Error("failed to get shared communities for gear", "error", err)
		return nil, connecterr.Internal(ctx, "ListUserGear", err)
	}

	// Convert storage models to API response items, skipping terminal-state gear.
	items := make([]*api.GearItem, 0, len(messages))
	var droppedCount int
	for _, msg := range messages {
		gearStored := msg.(*models.Gear)
		if gearStored.State == models.GearState_GEAR_STATE_GIVEN_AWAY {
			droppedCount++
			continue
		}
		gearState := convertGearState(gearStored.State)
		items = append(items, &api.GearItem{
			Id:                 gearStored.Id,
			Name:               gearStored.Name,
			Description:        gearStored.Description,
			Owner:              owner,
			LocationId:         gearStored.LocationId,
			MediaIds:           gearStored.MediaIds,
			SharedCommunityIds: sharedCommunityMap[gearStored.Id],
			ValueEstimate:      convertStorageValueEstimateToAPI(gearStored.ValueEstimate),
			CreatedAtUnixSec:   gearStored.CreatedAtUnixSec,
			State:              &gearState,
		})
	}
	if droppedCount > 0 {
		logger.DebugContext(ctx, "filtered terminal-state gear", "dropped_count", droppedCount)
	}

	res := connect.NewResponse(&api.ListUserGearResponse{
		Items: items,
	})
	return res, nil
}

// getSharedCommunityMap builds a map of gear_id -> []community_id for the given gear items.
func (s *Service) getSharedCommunityMap(ctx context.Context, gearMessages []proto.Message) (map[string][]string, error) {
	result := make(map[string][]string)

	gearIDs := make([]string, 0, len(gearMessages))
	for _, msg := range gearMessages {
		gear := msg.(*models.Gear)
		gearIDs = append(gearIDs, gear.Id)
	}

	if len(gearIDs) == 0 {
		return result, nil
	}

	communityGears, err := s.storage.QueryByFieldIn(ctx, "gear_id", gearIDs, &models.CommunityGear{})
	if err != nil {
		return nil, fmt.Errorf("failed to query community gear: %w", err)
	}

	for _, cg := range communityGears {
		communityGear := cg.(*models.CommunityGear)
		result[communityGear.GearId] = append(result[communityGear.GearId], communityGear.CommunityId)
	}

	return result, nil
}
