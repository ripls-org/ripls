package community

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	regionlib "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/storage"
)

// RegionResult represents a computed region for a community.
type RegionResult struct {
	RegionType       string
	RegionName       string
	RegionCode       string
	MemberPercentage float64
	IsOverride       bool
}

// ComputeRegions calculates the geographic regions for a community based on member locations.
//
// Algorithm:
// - Queries all community members' primary residence locations
// - For each administrative level (neighborhood, city, county, state):
//   - Counts occurrences of each value
//   - If ≥50% of members share the same value, creates a region entry
//
// - Returns all qualifying regions
//
// Example: If 8/10 members live in Boulder, CO:
//   - City entry: {type: "city", name: "Boulder, CO", percentage: 0.8}
//   - State entry: {type: "state", name: "Colorado", percentage: 1.0}
func ComputeRegions(ctx context.Context, storage *storage.ProtoSQLStorage, communityID string) ([]RegionResult, error) {
	// Get all members of the community
	members, err := getCommunityMembers(ctx, storage, communityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get community members: %w", err)
	}

	if len(members) == 0 {
		return []RegionResult{}, nil // No members, no regions
	}

	// Get locations for all members
	locations, err := getMemberLocations(ctx, storage, members)
	if err != nil {
		return nil, fmt.Errorf("failed to get member locations: %w", err)
	}

	if len(locations) == 0 {
		return []RegionResult{}, nil // No locations, no regions
	}

	// Compute regions at each administrative level
	var results []RegionResult

	// Neighborhood level
	if neighborhood, percentage := findMostCommon(locations, func(loc *models.Location) string {
		return loc.Address.GetNeighborhood()
	}); neighborhood != "" && percentage >= 0.5 {
		results = append(results, RegionResult{
			RegionType:       "neighborhood",
			RegionName:       neighborhood,
			MemberPercentage: percentage,
			IsOverride:       false,
		})
	}

	// City level
	if city, percentage := findMostCommon(locations, func(loc *models.Location) string {
		if loc.Address.Locality == "" {
			return ""
		}
		// Scope city with state if available for disambiguation
		if loc.Address.GetAdministrativeArea() != "" {
			return fmt.Sprintf("%s, %s", loc.Address.Locality, loc.Address.GetAdministrativeArea())
		}
		return loc.Address.Locality
	}); city != "" && percentage >= 0.5 {
		results = append(results, RegionResult{
			RegionType:       "city",
			RegionName:       city,
			MemberPercentage: percentage,
			IsOverride:       false,
		})
	}

	// County level
	if county, percentage := findMostCommon(locations, func(loc *models.Location) string {
		return loc.Address.GetCounty()
	}); county != "" && percentage >= 0.5 {
		results = append(results, RegionResult{
			RegionType:       "county",
			RegionName:       county,
			MemberPercentage: percentage,
			IsOverride:       false,
		})
	}

	// State level
	if state, percentage := findMostCommon(locations, func(loc *models.Location) string {
		return loc.Address.GetAdministrativeArea()
	}); state != "" && percentage >= 0.5 {
		results = append(results, RegionResult{
			RegionType:       "state",
			RegionName:       state,
			MemberPercentage: percentage,
			IsOverride:       false,
		})
	}

	return results, nil
}

// SaveRegions saves computed regions to the database, replacing any existing auto-computed regions.
// Manual overrides (is_override=true) are preserved.
func SaveRegions(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID string, regions []RegionResult) error {
	// Delete existing auto-computed regions for this community
	if err := deleteAutoComputedRegions(ctx, sqlStorage, communityID); err != nil {
		return fmt.Errorf("failed to delete existing regions: %w", err)
	}

	// Insert new regions
	now := time.Now().Unix()
	for _, region := range regions {
		// Get or create the normalized region
		regionID, err := regionlib.GetOrCreateRegion(
			ctx,
			sqlStorage,
			region.RegionType,
			region.RegionName,
			region.RegionCode,
			"", // parent_region_id computed internally
		)
		if err != nil {
			return fmt.Errorf("failed to get or create region: %w", err)
		}

		communityRegion := &models.CommunityRegion{
			Id:               uuid.New().String(),
			CommunityId:      communityID,
			RegionId:         regionID,
			MemberPercentage: region.MemberPercentage,
			IsOverride:       region.IsOverride,
			CreatedAtUnixSec: now,
			UpdatedAtUnixSec: now,
		}

		if _, err := sqlStorage.Insert(ctx, communityRegion); err != nil {
			return fmt.Errorf("failed to insert region: %w", err)
		}
	}

	return nil
}

// getCommunityMembers retrieves all members of a community.
func getCommunityMembers(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID string) ([]*models.CommunityUser, error) {
	membersRaw, err := sqlStorage.QueryByField(ctx, "community_id", communityID, &models.CommunityUser{})
	if err != nil {
		return nil, err
	}

	members := make([]*models.CommunityUser, 0, len(membersRaw))
	for _, msg := range membersRaw {
		members = append(members, msg.(*models.CommunityUser))
	}
	return members, nil
}

// getMemberLocations retrieves primary residence locations for all members.
func getMemberLocations(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, members []*models.CommunityUser) ([]*models.Location, error) {
	var locations []*models.Location

	for _, member := range members {
		// Get user to access primary_residence_location_id
		user := &models.User{}
		if err := sqlStorage.GetByID(ctx, member.UserId, user); err != nil {
			// Skip members without user records (shouldn't happen but be defensive)
			continue
		}

		if user.PrimaryResidenceLocationId == "" {
			// Skip members without primary residence
			continue
		}

		// Get the location
		location := &models.Location{}
		if err := sqlStorage.GetByID(ctx, user.PrimaryResidenceLocationId, location); err != nil {
			// Skip members with invalid location IDs
			continue
		}

		locations = append(locations, location)
	}

	return locations, nil
}

// findMostCommon finds the most common value in a set of locations using the provided extractor function.
// Returns the most common value and its percentage (0.0-1.0).
// Returns empty string and 0 if no values are found.
func findMostCommon(locations []*models.Location, extract func(*models.Location) string) (string, float64) {
	if len(locations) == 0 {
		return "", 0
	}

	// Count occurrences of each value
	counts := make(map[string]int)
	for _, loc := range locations {
		value := extract(loc)
		if value != "" {
			counts[value]++
		}
	}

	if len(counts) == 0 {
		return "", 0
	}

	// Find the most common value
	var mostCommon string
	maxCount := 0
	for value, count := range counts {
		if count > maxCount {
			mostCommon = value
			maxCount = count
		}
	}

	// Calculate percentage
	percentage := float64(maxCount) / float64(len(locations))
	return mostCommon, percentage
}

// deleteAutoComputedRegions deletes all auto-computed regions for a community.
// Manual overrides (is_override=true) are preserved.
func deleteAutoComputedRegions(ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID string) error {
	// Query for auto-computed regions
	regionsRaw, err := sqlStorage.QueryByFields(ctx, map[string]any{
		"community_id": communityID,
		"is_override":  false,
	}, &models.CommunityRegion{})
	if err != nil {
		return err
	}

	// Delete each auto-computed region
	for _, msg := range regionsRaw {
		region := msg.(*models.CommunityRegion)
		if err := sqlStorage.Delete(ctx, region); err != nil {
			return err
		}
	}

	return nil
}
