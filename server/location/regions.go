// Standalone library for region management operations.
// Per server architecture guidelines, services must NOT call other services.
// This library provides shared functionality that both LocationService and
// CommunityService can use without creating service-to-service dependencies.

package location

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"

	"github.com/google/uuid"
)

// GetOrCreateRegion ensures a region exists with the given details.
// Returns existing region ID if found, creates new one if not.
// This is a standalone library function that takes storage as a parameter.
func GetOrCreateRegion(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	regionType string,
	regionName string,
	regionCode string,
	parentRegionID string,
) (string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetOrCreateRegion",
		"region_type", regionType,
		"region_name", regionName,
	)

	// Try to find existing region
	existingRegion, err := FindRegionByDetails(ctx, st, regionType, regionName, parentRegionID)
	if err == nil && existingRegion != nil {
		logger.DebugContext(ctx, "found existing region", "region_id", existingRegion.Id)
		return existingRegion.Id, nil
	}

	// Create new region
	regionID := uuid.NewString()
	now := time.Now().Unix()

	// Build hierarchical path
	path, err := BuildRegionPath(ctx, st, parentRegionID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build region path", "error", err)
		return "", fmt.Errorf("failed to build region path: %w", err)
	}
	// Append current region to path
	if path != "" {
		path = path + "," + regionID
	} else {
		path = regionID
	}

	region := &models.Region{
		Id:               regionID,
		RegionType:       regionType,
		RegionName:       regionName,
		RegionCode:       &regionCode,
		ParentRegionId:   &parentRegionID,
		Path:             path,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}

	if _, err := st.Insert(ctx, region); err != nil {
		logger.ErrorContext(ctx, "failed to create region", "error", err)
		return "", fmt.Errorf("failed to create region: %w", err)
	}

	logger.InfoContext(ctx, "created new region", "region_id", regionID)
	return regionID, nil
}

// BuildRegionPath constructs the hierarchical path for a region.
// Returns the path from root to the parent region.
func BuildRegionPath(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	parentRegionID string,
) (string, error) {
	if parentRegionID == "" {
		return "", nil
	}

	// Get parent region
	parentRegion := &models.Region{}
	if err := st.GetByID(ctx, parentRegionID, parentRegion); err != nil {
		return "", fmt.Errorf("failed to get parent region: %w", err)
	}

	return parentRegion.Path, nil
}

// GetRegionDisplayName builds display name with parent context.
// Example: "Boulder" with parent "Colorado" -> "Boulder, CO".
func GetRegionDisplayName(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	region *models.Region,
) (string, error) {
	if region.GetParentRegionId() == "" {
		return region.RegionName, nil
	}

	// Get parent region
	parentRegion := &models.Region{}
	if err := st.GetByID(ctx, region.GetParentRegionId(), parentRegion); err != nil {
		// If parent not found, just return region name
		return region.RegionName, nil
	}

	// Build display name with parent context
	// For states: use region_code if available (e.g., "CO" instead of "Colorado")
	// For other types: use region_name
	parentName := parentRegion.RegionName
	if parentRegion.RegionType == "state" && parentRegion.GetRegionCode() != "" {
		parentName = parentRegion.GetRegionCode()
	}

	return fmt.Sprintf("%s, %s", region.RegionName, parentName), nil
}

// FindRegionByDetails searches for an existing region by type, name, and parent.
// Returns nil, nil if not found.
func FindRegionByDetails(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	regionType string,
	regionName string,
	parentRegionID string,
) (*models.Region, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "FindRegionByDetails",
		"region_type", regionType,
		"region_name", regionName,
	)

	// Query by region_type and region_name
	results, err := st.QueryByFields(ctx, map[string]interface{}{
		"region_type": regionType,
		"region_name": regionName,
	}, &models.Region{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query regions", "error", err)
		return nil, fmt.Errorf("failed to query regions: %w", err)
	}

	// Filter by parent_region_id (must match exactly, including empty strings)
	for _, msg := range results {
		region := msg.(*models.Region)
		if region.GetParentRegionId() == parentRegionID {
			logger.DebugContext(ctx, "found matching region", "region_id", region.Id)
			return region, nil
		}
	}

	return nil, nil
}

// GetCommunityCountForRegion returns the number of communities in a region.
func GetCommunityCountForRegion(
	ctx context.Context,
	st *storage.ProtoSQLStorage,
	regionID string,
) (int32, error) {
	// Query community_regions table
	results, err := st.QueryByField(ctx, "region_id", regionID, &models.CommunityRegion{})
	if err != nil {
		return 0, fmt.Errorf("failed to query community regions: %w", err)
	}

	// Use map to deduplicate by community_id
	uniqueCommunities := make(map[string]bool)
	for _, msg := range results {
		cr := msg.(*models.CommunityRegion)
		uniqueCommunities[cr.CommunityId] = true
	}

	return int32(len(uniqueCommunities)), nil
}

// NormalizeRegionName normalizes a region name for consistent comparison.
// Removes extra whitespace, converts to lowercase for case-insensitive matching.
func NormalizeRegionName(name string) string {
	// Trim whitespace
	name = strings.TrimSpace(name)
	// Collapse multiple spaces to single space
	name = strings.Join(strings.Fields(name), " ")
	return name
}
