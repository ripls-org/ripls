package location

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// findLocationByExternalPlaceID finds an existing location by its external
// place identifier (provider-scoped). Returns nil if no matching location
// is found. Both id and provider must be non-empty for a match; the pair
// together identifies a canonical place.
func (s *Service) findLocationByExternalPlaceID(ctx context.Context, externalID, provider string) (*models.Location, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "findLocationByExternalPlaceID",
		"external_place_id", externalID,
		"external_place_provider", provider,
	)

	// Query locations with matching (external_place_id, external_place_provider).
	// The provider filter prevents ID collisions between different geocoding
	// services from being treated as the same place.
	results, err := s.storage.QueryByFields(
		ctx,
		map[string]any{
			"external_place_id":       externalID,
			"external_place_provider": provider,
		},
		&models.Location{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query by external place ID", "error", err)
		return nil, fmt.Errorf("failed to query locations: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	loc := results[0].(*models.Location)
	logger.InfoContext(ctx, "found existing location by external place ID", "location_id", loc.Id)
	return loc, nil
}

// findNearbyLocation finds an existing location within a radius using coordinate proximity.
// This is a fallback when Mapbox feature ID is not available.
// Returns nil if no nearby location is found within the radius.
func (s *Service) findNearbyLocation(ctx context.Context, latitudeDeg, longitudeDeg, radiusMeters float64) (*models.Location, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "findNearbyLocation",
		"latitude", latitudeDeg,
		"longitude", longitudeDeg,
		"radius_meters", radiusMeters,
	)

	// Use QueryByProximity to find locations within the radius
	results, err := s.storage.QueryByProximity(
		ctx,
		latitudeDeg, longitudeDeg, radiusMeters,
		&models.Location{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query locations by proximity", "error", err)
		return nil, fmt.Errorf("failed to query locations: %w", err)
	}

	// QueryByProximity returns results sorted by distance (ascending)
	// Return the closest location if any were found
	if len(results) > 0 {
		closestLocation := results[0].Message.(*models.Location)
		logger.InfoContext(ctx, "found nearby location",
			"location_id", closestLocation.Id,
			"distance_meters", results[0].DistanceMeters,
		)
		return closestLocation, nil
	}

	logger.DebugContext(ctx, "no nearby location found within radius")
	return nil, nil
}

// findOrCreateLocation checks for duplicate locations and returns an existing
// one if found, or returns nil if a new location should be created.
//
// Dedup strategy:
//  1. If (externalID, externalProvider) is set, look up by that pair — this
//     is the canonical identity for places from a geocoding provider.
//  2. Otherwise, fall back to coordinate proximity (50-meter radius) for
//     locations that weren't created via a provider lookup.
func (s *Service) findOrCreateLocation(
	ctx context.Context,
	userID string,
	externalID, externalProvider string,
	latitudeDeg, longitudeDeg float64,
) (*models.Location, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "findOrCreateLocation",
		"user_id", userID,
		"external_place_id", externalID,
		"external_place_provider", externalProvider,
	)

	// Step 1: Check for existing location with same external place ID+provider.
	if externalID != "" && externalProvider != "" {
		existingLocation, err := s.findLocationByExternalPlaceID(ctx, externalID, externalProvider)
		if err != nil {
			logger.ErrorContext(ctx, "failed to check for existing location by external ID", "error", err)
			return nil, fmt.Errorf("failed to check for existing location: %w", err)
		}
		if existingLocation != nil {
			logger.InfoContext(ctx, "found existing location via external place ID",
				"location_id", existingLocation.Id,
				"dedup_path", "external_id",
			)
			return existingLocation, nil
		}
	} else {
		// Step 2: Fallback to coordinate proximity (no external ID to match on).
		logger.InfoContext(ctx, "no external place ID provided, using coordinate proximity fallback")

		existingLocation, err := s.findNearbyLocation(
			ctx,
			latitudeDeg,
			longitudeDeg,
			50.0, // 50 meter threshold
		)
		if err != nil {
			logger.ErrorContext(ctx, "failed to check for nearby location", "error", err)
			return nil, fmt.Errorf("failed to check for nearby location: %w", err)
		}
		if existingLocation != nil {
			logger.InfoContext(ctx, "found existing location via coordinate proximity",
				"location_id", existingLocation.Id,
				"dedup_path", "proximity",
			)
			return existingLocation, nil
		}
	}

	// No duplicate found.
	logger.DebugContext(ctx, "no existing location found, should create new location",
		"dedup_path", "new")
	return nil, nil
}
