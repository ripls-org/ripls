package location

import (
	"context"
	"fmt"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GetUserLocationFallback retrieves a location for the user using the fallback hierarchy:
// 1. User's primary residence (if set)
// 2. Most recent location from user_locations (by last_used_at)
// 3. nil if no locations found
//
// Returns the location ID and a GeocodedLocation with a friendly display name.
// If the location has no name, generates one from address components.
func GetUserLocationFallback(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
) (locationID string, geocoded *api.GeocodedLocation, err error) {
	logger := logging.LoggerWithContext(ctx)

	// First try to get primary residence from user record
	user := &models.User{}
	err = store.GetByID(ctx, userID, user)
	var fallbackLocationID string

	if err == nil && user.PrimaryResidenceLocationId != "" {
		fallbackLocationID = user.PrimaryResidenceLocationId
		logger.Debug("using primary residence for fallback", "location_id", fallbackLocationID)
	} else {
		if err != nil {
			logger.Warn("failed to get user for fallback location resolution", "error", err)
		} else {
			logger.Warn("user has no primary residence set for fallback")
		}

		// Query user_locations to get most recent location
		userLocations, err := store.QueryByField(
			ctx,
			"user_id",
			userID,
			&models.UserLocation{},
		)
		if err != nil {
			logger.Warn("failed to query user_locations for fallback", "error", err)
		} else if len(userLocations) > 0 {
			// Find the most recent location (highest last_used_at)
			var mostRecent *models.UserLocation
			for _, ul := range userLocations {
				userLoc := ul.(*models.UserLocation)
				if mostRecent == nil || userLoc.LastUsedAtUnixSec > mostRecent.LastUsedAtUnixSec {
					mostRecent = userLoc
				}
			}
			if mostRecent != nil {
				fallbackLocationID = mostRecent.LocationId
				logger.Debug("using most recent location for fallback", "location_id", fallbackLocationID)
			}
		}

		if fallbackLocationID == "" {
			logger.Warn("user has no locations at all for fallback")
			return "", nil, nil
		}
	}

	// Fetch and convert the fallback location
	if fallbackLocationID != "" {
		loc := &models.Location{}
		if err := store.GetByID(ctx, fallbackLocationID, loc); err != nil {
			logger.Warn("failed to get fallback location", "location_id", fallbackLocationID, "error", err)
			return "", nil, fmt.Errorf("failed to get fallback location: %w", err)
		}

		// Generate a friendly name if the location has no name
		locationName := GenerateLocationDisplayName(loc)
		if locationName != loc.GetName() && loc.GetName() == "" {
			logger.Debug("generated display name for location with no name", "location_id", fallbackLocationID, "generated_name", locationName)
		}

		geocodedLocation := &api.GeocodedLocation{
			Name:         locationName,
			LatitudeDeg:  loc.Geolocation.LatitudeDeg,
			LongitudeDeg: loc.Geolocation.LongitudeDeg,
			Locality:     loc.Address.Locality,
			RegionCode:   loc.Address.RegionCode,
			PostalCode:   loc.Address.PostalCode,
			AddressLines: loc.Address.AddressLines,
		}

		logger.Info("using user location as fallback", "name", locationName, "is_primary", fallbackLocationID == user.PrimaryResidenceLocationId)
		return fallbackLocationID, geocodedLocation, nil
	}

	return "", nil, nil
}

// GetUserLocationFallbackLogged is a fire-and-forget variant of GetUserLocationFallback
// that never returns an error. The underlying function already logs every failure path at
// Warn level; this wrapper surfaces any future error paths as a defensive Warn log so
// systemic storage issues on this hot path remain visible. Use this from call sites that
// have no recovery path beyond "no fallback location available.".
func GetUserLocationFallbackLogged(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID string,
) (locationID string, geocoded *api.GeocodedLocation) {
	locationID, geocoded, err := GetUserLocationFallback(ctx, store, userID)
	if err != nil {
		logging.LoggerWithContext(ctx).Warn(
			"GetUserLocationFallback returned error",
			"user_id", userID,
			"error", err,
		)
	}
	return locationID, geocoded
}

// GenerateLocationDisplayName generates a friendly display name for a location.
// Priority:
// 1. Location's existing name (if set)
// 2. First address line (most specific)
// 3. "Locality, RegionCode" (city/state)
// 4. "Your Location" (ultimate fallback).
func GenerateLocationDisplayName(loc *models.Location) string {
	// If location already has a name, use it
	if loc.GetName() != "" {
		return loc.GetName()
	}

	// Build name from address components with priority:
	// 1. First address line (most specific)
	// 2. "Locality, RegionCode" (city/state)
	// 3. "Your Location" (ultimate fallback)
	if len(loc.Address.AddressLines) > 0 && loc.Address.AddressLines[0] != "" {
		return loc.Address.AddressLines[0]
	}

	if loc.Address.Locality != "" {
		locationName := loc.Address.Locality
		if loc.Address.RegionCode != "" {
			locationName += ", " + loc.Address.RegionCode
		}
		return locationName
	}

	return "Your Location"
}
