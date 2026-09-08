package community

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	regionlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
)

// ListCommunityRegions lists all regions associated with a community.
func (s *Service) ListCommunityRegions(
	ctx context.Context,
	req *connect.Request[api.ListCommunityRegionsRequest],
) (*connect.Response[api.ListCommunityRegionsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ListCommunityRegions",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
	)

	// Reject if community is missing or soft-deleted.
	if _, err := auth.RequireActiveCommunity(ctx, s.storage, req.Msg.CommunityId); err != nil {
		return nil, err
	}

	// Query all regions for this community
	regionsRaw, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityRegion{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query regions", "error", err)
		return nil, connecterr.Internal(ctx, "ListCommunityRegions", fmt.Errorf("failed to query regions"))
	}

	// Convert to API format
	apiRegions := make([]*api.CommunityRegionItem, 0, len(regionsRaw))
	for _, msg := range regionsRaw {
		region := msg.(*models.CommunityRegion)
		apiRegions = append(apiRegions, &api.CommunityRegionItem{
			RegionId:         region.RegionId,
			MemberPercentage: region.MemberPercentage,
			IsOverride:       region.IsOverride,
		})
	}

	logger.InfoContext(ctx, "listed community regions", "region_count", len(apiRegions))

	return connect.NewResponse(&api.ListCommunityRegionsResponse{
		Regions: apiRegions,
	}), nil
}

// SetCommunityRegionOverride manually sets a region override for a community.
func (s *Service) SetCommunityRegionOverride(
	ctx context.Context,
	req *connect.Request[api.SetCommunityRegionOverrideRequest],
) (*connect.Response[api.SetCommunityRegionOverrideResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "SetCommunityRegionOverride",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", req.Msg.CommunityId,
		"region_id", req.Msg.RegionId,
	)

	// Reject if community is missing, soft-deleted, or caller is not an active member.
	community, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	if community.OwnerUserId != authInfo.UserID {
		logger.WarnContext(ctx, "user is not community owner")
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "community_owner_required_for_region_override", "only the current community owner can set the region override", nil)
	}

	// Verify region exists
	region := &models.Region{}
	if err := s.storage.GetByID(ctx, req.Msg.RegionId, region); err != nil {
		logger.ErrorContext(ctx, "region not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("region not found"))
	}

	// Delete all existing region associations for this community
	existingRegions, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityRegion{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing regions", "error", err)
		return nil, connecterr.Internal(ctx, "SetCommunityRegionOverride", fmt.Errorf("failed to query existing regions"))
	}

	for _, msg := range existingRegions {
		cr := msg.(*models.CommunityRegion)
		if err := s.storage.Delete(ctx, cr); err != nil {
			logger.ErrorContext(ctx, "failed to delete existing region", "region_id", cr.Id, "error", err)
			return nil, connecterr.Internal(ctx, "SetCommunityRegionOverride", fmt.Errorf("failed to delete existing region"))
		}
	}

	// Create new region override
	now := clock.UnixSec(ctx)
	communityRegion := &models.CommunityRegion{
		Id:               uuid.NewString(),
		CommunityId:      req.Msg.CommunityId,
		RegionId:         req.Msg.RegionId,
		MemberPercentage: 1.0, // 100% for manual override
		IsOverride:       true,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}

	if _, err := s.storage.Insert(ctx, communityRegion); err != nil {
		logger.ErrorContext(ctx, "failed to create region override", "error", err)
		return nil, connecterr.Internal(ctx, "SetCommunityRegionOverride", fmt.Errorf("failed to create region override"))
	}

	logger.InfoContext(ctx, "set community region override")

	return connect.NewResponse(&api.SetCommunityRegionOverrideResponse{}), nil
}

// recomputeCommunityRegions recomputes regions for a community based on member locations.
// This is called after membership changes or location updates.
func (s *Service) recomputeCommunityRegions(ctx context.Context, communityID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "recomputeCommunityRegions",
		"community_id", communityID,
	)

	// Check if there's a manual override - if so, don't recompute
	existingRegions, err := s.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityRegion{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query existing regions", "error", err)
		return fmt.Errorf("failed to query existing regions: %w", err)
	}

	for _, msg := range existingRegions {
		cr := msg.(*models.CommunityRegion)
		if cr.IsOverride {
			logger.DebugContext(ctx, "skipping recompute due to manual override")
			return nil
		}
	}

	// Get all community members
	membersRaw, err := s.storage.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query members", "error", err)
		return fmt.Errorf("failed to query members: %w", err)
	}

	if len(membersRaw) == 0 {
		logger.DebugContext(ctx, "no members found, skipping region computation")
		return nil
	}

	// Get member locations and extract regions
	regionCounts := make(map[string]int) // region_id -> count
	totalMembers := len(membersRaw)

	for _, msg := range membersRaw {
		member := msg.(*models.CommunityUser)

		// Get user
		user := &models.User{}
		if err := s.storage.GetByID(ctx, member.UserId, user); err != nil {
			continue
		}

		// Skip if user has no location
		if user.PrimaryResidenceLocationId == "" {
			continue
		}

		// Get location details
		location := &models.Location{}
		if err := s.storage.GetByID(ctx, user.PrimaryResidenceLocationId, location); err != nil {
			continue
		}

		// Extract regions from address (neighborhood, city, county, state)
		// For each region type, find or create the region
		if location.Address.GetNeighborhood() != "" {
			// Neighborhood region
			neighborhoodRegionID, err := regionlib.GetOrCreateRegion(
				ctx,
				s.storage,
				"neighborhood",
				location.Address.GetNeighborhood(),
				"",
				"", // Could parent to city if we track hierarchy
			)
			if err == nil {
				regionCounts[neighborhoodRegionID]++
			}
		}

		if location.Address.Locality != "" {
			// City region
			cityRegionID, err := regionlib.GetOrCreateRegion(
				ctx,
				s.storage,
				"city",
				location.Address.Locality,
				"",
				"", // parent will be state if we have administrative_area
			)
			if err == nil {
				regionCounts[cityRegionID]++
			}
		}

		if location.Address.GetCounty() != "" {
			// County region
			countyRegionID, err := regionlib.GetOrCreateRegion(
				ctx,
				s.storage,
				"county",
				location.Address.GetCounty(),
				"",
				"",
			)
			if err == nil {
				regionCounts[countyRegionID]++
			}
		}

		if location.Address.GetAdministrativeArea() != "" {
			// State/province region
			stateRegionID, err := regionlib.GetOrCreateRegion(
				ctx,
				s.storage,
				"state",
				location.Address.GetAdministrativeArea(),
				"", // Could extract state code if available
				"",
			)
			if err == nil {
				regionCounts[stateRegionID]++
			}
		}
	}

	// Delete existing auto-computed regions
	for _, msg := range existingRegions {
		cr := msg.(*models.CommunityRegion)
		if !cr.IsOverride {
			if err := s.storage.Delete(ctx, cr); err != nil {
				logger.ErrorContext(ctx, "failed to delete old region", "region_id", cr.Id, "error", err)
				return fmt.Errorf("failed to delete old region %s: %w", cr.Id, err)
			}
		}
	}

	// Create new region associations
	now := clock.UnixSec(ctx)
	for regionID, count := range regionCounts {
		memberPercentage := float64(count) / float64(totalMembers)

		communityRegion := &models.CommunityRegion{
			Id:               uuid.NewString(),
			CommunityId:      communityID,
			RegionId:         regionID,
			MemberPercentage: memberPercentage,
			IsOverride:       false,
			CreatedAtUnixSec: now,
			UpdatedAtUnixSec: now,
		}

		if _, err := s.storage.Insert(ctx, communityRegion); err != nil {
			logger.ErrorContext(ctx, "failed to create region association", "region_id", regionID, "error", err)
			continue
		}
	}

	logger.InfoContext(ctx, "recomputed community regions", "region_count", len(regionCounts))

	return nil
}
