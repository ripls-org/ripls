package location

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	regionlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
)

// SearchRegions searches for regions by name (typeahead search).
func (s *Service) SearchRegions(
	ctx context.Context,
	req *connect.Request[api.SearchRegionsRequest],
) (*connect.Response[api.SearchRegionsResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "SearchRegions",
		"query", req.Msg.Query,
		"region_type", req.Msg.RegionType,
	)

	// Normalize query for case-insensitive search
	query := strings.TrimSpace(strings.ToLower(req.Msg.Query))

	// Query regions by type (or all types if not specified)
	var results []proto.Message
	var err error
	if req.Msg.RegionType != "" {
		// Query specific region type
		results, err = s.storage.QueryByField(ctx, "region_type", req.Msg.RegionType, &models.Region{})
		if err != nil {
			logger.ErrorContext(ctx, "failed to query regions", "error", err)
			return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to query regions")
		}
	} else {
		// Query all region types
		regionTypes := []string{"city", "county", "state", "country", "neighborhood"}
		for _, regionType := range regionTypes {
			typeResults, err := s.storage.QueryByField(ctx, "region_type", regionType, &models.Region{})
			if err != nil {
				logger.ErrorContext(ctx, "failed to query region type", "region_type", regionType, "error", err)
				return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to query region type %s")
			}
			results = append(results, typeResults...)
		}
	}

	// Filter by query string and build region items
	type regionWithScore struct {
		item  *api.RegionItem
		score int // For sorting by relevance
	}
	var matchedRegions []regionWithScore

	for _, msg := range results {
		region := msg.(*models.Region)

		// Filter by query if provided
		if query != "" {
			regionNameLower := strings.ToLower(region.RegionName)
			if !strings.Contains(regionNameLower, query) {
				continue
			}

			// Calculate relevance score (exact match = highest). Every branch
			// assigns, so declare without an initial value the code never reads.
			var score int
			switch {
			case regionNameLower == query:
				score = 100
			case strings.HasPrefix(regionNameLower, query):
				score = 50
			default:
				score = 10
			}

			// Get community count for this region
			communityCount, err := regionlib.GetCommunityCountForRegion(ctx, s.storage, region.Id)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get community count", "region_id", region.Id, "error", err)
				return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to get community count for region %s")
			}

			// Boost score by community count
			score += int(communityCount)

			// Build display name with parent context
			displayName, err := regionlib.GetRegionDisplayName(ctx, s.storage, region)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get display name", "region_id", region.Id, "error", err)
				return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to get display name for region %s")
			}

			matchedRegions = append(matchedRegions, regionWithScore{
				item: &api.RegionItem{
					RegionId:       region.Id,
					DisplayName:    displayName,
					RegionType:     region.RegionType,
					CommunityCount: communityCount,
				},
				score: score,
			})
		} else {
			// No query filter, include all
			communityCount, err := regionlib.GetCommunityCountForRegion(ctx, s.storage, region.Id)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get community count", "region_id", region.Id, "error", err)
				return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to get community count for region %s")
			}

			displayName, err := regionlib.GetRegionDisplayName(ctx, s.storage, region)
			if err != nil {
				logger.ErrorContext(ctx, "failed to get display name", "region_id", region.Id, "error", err)
				return nil, connecterr.Internal(ctx, "SearchRegions", err, "detail", "failed to get display name for region %s")
			}

			matchedRegions = append(matchedRegions, regionWithScore{
				item: &api.RegionItem{
					RegionId:       region.Id,
					DisplayName:    displayName,
					RegionType:     region.RegionType,
					CommunityCount: communityCount,
				},
				score: int(communityCount),
			})
		}
	}

	// Sort by relevance score descending
	sort.Slice(matchedRegions, func(i, j int) bool {
		return matchedRegions[i].score > matchedRegions[j].score
	})

	// Extract region items
	regionItems := make([]*api.RegionItem, len(matchedRegions))
	for i, r := range matchedRegions {
		regionItems[i] = r.item
	}

	logger.InfoContext(ctx, "successfully searched regions", "count", len(regionItems))

	return connect.NewResponse(&api.SearchRegionsResponse{
		Regions: regionItems,
	}), nil
}

// GetRegion retrieves a region by ID.
func (s *Service) GetRegion(
	ctx context.Context,
	req *connect.Request[api.GetRegionRequest],
) (*connect.Response[api.GetRegionResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetRegion",
		"region_id", req.Msg.RegionId,
	)

	// Get region
	region := &models.Region{}
	if err := s.storage.GetByID(ctx, req.Msg.RegionId, region); err != nil {
		logger.ErrorContext(ctx, "region not found", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("region not found: %w", err))
	}

	// Get community count
	communityCount, err := regionlib.GetCommunityCountForRegion(ctx, s.storage, region.Id)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get community count", "error", err)
		return nil, connecterr.Internal(ctx, "GetRegion", err, "detail",

			// Build display name
			"failed to get community count for region %s")
	}

	displayName, err := regionlib.GetRegionDisplayName(ctx, s.storage, region)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get display name", "error", err)
		return nil, connecterr.Internal(ctx, "GetRegion", err, "detail",

			// Build parent hierarchy
			"failed to get display name for region %s")
	}

	var parents []*api.RegionParent
	if region.GetParentRegionId() != "" {
		parentRegion := &models.Region{}
		if err := s.storage.GetByID(ctx, region.GetParentRegionId(), parentRegion); err == nil {
			parents = append(parents, &api.RegionParent{
				RegionId:   parentRegion.Id,
				RegionName: parentRegion.RegionName,
				RegionType: parentRegion.RegionType,
			})

			// Get grandparent if exists
			if parentRegion.GetParentRegionId() != "" {
				grandparentRegion := &models.Region{}
				if err := s.storage.GetByID(ctx, parentRegion.GetParentRegionId(), grandparentRegion); err == nil {
					parents = append(parents, &api.RegionParent{
						RegionId:   grandparentRegion.Id,
						RegionName: grandparentRegion.RegionName,
						RegionType: grandparentRegion.RegionType,
					})
				}
			}
		}
	}

	regionItem := &api.RegionItem{
		RegionId:       region.Id,
		DisplayName:    displayName,
		RegionType:     region.RegionType,
		CommunityCount: communityCount,
		Parents:        parents,
	}

	logger.InfoContext(ctx, "successfully retrieved region")

	return connect.NewResponse(&api.GetRegionResponse{
		Region: regionItem,
	}), nil
}

// CreateRegionFromAddress creates or finds a region based on address details.
// This is used by the community governance flow to create regions for manual override.
func (s *Service) CreateRegionFromAddress(
	ctx context.Context,
	req *connect.Request[api.CreateRegionFromAddressRequest],
) (*connect.Response[api.CreateRegionFromAddressResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateRegionFromAddress",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"preferred_type", req.Msg.PreferredRegionType,
	)

	// Validate required fields
	if req.Msg.Locality == "" && req.Msg.County == "" && req.Msg.AdministrativeArea == "" {
		logger.WarnContext(ctx, "address must contain at least locality, county, or administrative_area")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address must contain at least locality, county, or administrative_area"))
	}

	// Determine region type and name from address components
	// Priority: neighborhood > city > county > state
	var regionType, regionName, regionCode, parentRegionID string

	// Compute the state code once and reuse across all branches that need it.
	// Returns "" for non-US or unrecognized names — those callers still create
	// the state region with region_name = the full string, matching today's
	// behavior. A Warn log below surfaces any unexpected empty result.
	stateCode := regionlib.ExtractStateCode(req.Msg.AdministrativeArea)
	if req.Msg.AdministrativeArea != "" && stateCode == "" {
		logger.WarnContext(ctx, "unrecognized administrative_area, storing state region with empty code",
			"administrative_area", req.Msg.AdministrativeArea)
	}

	if req.Msg.Neighborhood != "" && req.Msg.PreferredRegionType == "neighborhood" {
		regionType = "neighborhood"
		regionName = req.Msg.Neighborhood
		// Parent is city
		if req.Msg.Locality != "" {
			cityRegionID, err := regionlib.GetOrCreateRegion(ctx, s.storage, "city", req.Msg.Locality, "", "")
			if err != nil {
				logger.ErrorContext(ctx, "failed to create parent city region", "error", err)
				return nil, connecterr.Internal(ctx, "CreateRegionFromAddress", fmt.Errorf("failed to create parent region"))
			}
			parentRegionID = cityRegionID
		}
	} else if req.Msg.Locality != "" {
		// City level (most common)
		regionType = "city"
		regionName = req.Msg.Locality
		// Scope city name with state code if available
		if stateCode != "" {
			regionName = fmt.Sprintf("%s, %s", req.Msg.Locality, stateCode)
		}
		// Parent is state
		if req.Msg.AdministrativeArea != "" {
			stateRegionID, err := regionlib.GetOrCreateRegion(ctx, s.storage, "state", req.Msg.AdministrativeArea, stateCode, "")
			if err != nil {
				logger.ErrorContext(ctx, "failed to create parent state region", "error", err)
				return nil, connecterr.Internal(ctx, "CreateRegionFromAddress", fmt.Errorf("failed to create parent region"))
			}
			parentRegionID = stateRegionID
		}
	} else if req.Msg.County != "" {
		// County level
		regionType = "county"
		regionName = req.Msg.County
		// Parent is state
		if req.Msg.AdministrativeArea != "" {
			stateRegionID, err := regionlib.GetOrCreateRegion(ctx, s.storage, "state", req.Msg.AdministrativeArea, stateCode, "")
			if err != nil {
				logger.ErrorContext(ctx, "failed to create parent state region", "error", err)
				return nil, connecterr.Internal(ctx, "CreateRegionFromAddress", fmt.Errorf("failed to create parent region"))
			}
			parentRegionID = stateRegionID
		}
	} else if req.Msg.AdministrativeArea != "" {
		// State level
		regionType = "state"
		regionName = req.Msg.AdministrativeArea
		regionCode = stateCode
	}

	logger = logger.With(
		"region_type", regionType,
		"region_name", regionName,
	)

	// Create or find the region using existing library
	regionID, err := regionlib.GetOrCreateRegion(ctx, s.storage, regionType, regionName, regionCode, parentRegionID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create or find region", "error", err)
		return nil, connecterr.Internal(ctx, "CreateRegionFromAddress", fmt.Errorf("failed to create region"))
	}

	logger.InfoContext(ctx, "created or found region", "region_id", regionID)

	return connect.NewResponse(&api.CreateRegionFromAddressResponse{
		RegionId:   regionID,
		RegionType: regionType,
		RegionName: regionName,
	}), nil
}
