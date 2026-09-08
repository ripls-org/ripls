package location

import (
	"context"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GetUserLocations retrieves user's locations sorted with primary location first, then by last_used_at (most recent first).
func (s *Service) GetUserLocations(
	ctx context.Context,
	req *connect.Request[api.GetUserLocationsRequest],
) (*connect.Response[api.GetUserLocationsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "getting user locations")

	// Set default limit
	limit := req.Msg.Limit
	if limit == 0 {
		limit = 50
	}

	// Get user's primary residence location ID
	user := &models.User{}
	err = s.storage.GetByID(ctx, authInfo.UserID, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user", "error", err)
		return nil, connecterr.Internal(ctx, "GetUserLocations", fmt.Errorf("failed to get user"))
	}

	primaryLocationID := user.PrimaryResidenceLocationId

	// Query user_locations table for authenticated user
	userLocations, err := s.storage.QueryByField(
		ctx,
		"user_id",
		authInfo.UserID,
		&models.UserLocation{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query user locations", "error", err)
		return nil, connecterr.Internal(ctx, "GetUserLocations", fmt.Errorf("failed to get user locations"))
	}

	// Enrich with full location details
	locations := []*api.UserLocationWithDetails{}
	for _, ul := range userLocations {
		userLoc := ul.(*models.UserLocation)

		// Fetch full location details
		loc := &models.Location{}
		err := s.storage.GetByID(ctx, userLoc.LocationId, loc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to get location details",
				"location_id", userLoc.LocationId,
				"error", err,
			)
			return nil, connecterr.Internal(ctx, "GetUserLocations", err, "detail", "failed to get location details for %s")
		}

		// Skip soft-deleted locations
		if loc.Deleted != nil {
			continue
		}

		locations = append(locations, &api.UserLocationWithDetails{
			LocationId:        userLoc.LocationId,
			LastUsedAtUnixSec: userLoc.LastUsedAtUnixSec,
			Location: &api.Location{
				Id:                 loc.Id,
				LatitudeDeg:        loc.Geolocation.LatitudeDeg,
				LongitudeDeg:       loc.Geolocation.LongitudeDeg,
				RegionCode:         loc.Address.RegionCode,
				PostalCode:         loc.Address.PostalCode,
				Locality:           loc.Address.Locality,
				AddressLines:       loc.Address.AddressLines,
				Name:               loc.Name,
				Neighborhood:       loc.Address.Neighborhood,
				County:             loc.Address.County,
				AdministrativeArea: loc.Address.AdministrativeArea,
			},
		})
	}

	// Sort: primary location first, then by last_used_at (most recent first)
	sort.Slice(locations, func(i, j int) bool {
		// Primary location always comes first
		if locations[i].LocationId == primaryLocationID {
			return true
		}
		if locations[j].LocationId == primaryLocationID {
			return false
		}
		// Otherwise sort by last_used_at (most recent first)
		return locations[i].LastUsedAtUnixSec > locations[j].LastUsedAtUnixSec
	})

	// Apply limit
	if int32(len(locations)) > limit {
		locations = locations[:limit]
	}

	logger.InfoContext(ctx, "retrieved user locations", "count", len(locations))

	resp := &api.GetUserLocationsResponse{
		Locations: locations,
	}

	return connect.NewResponse(resp), nil
}

// AddUserLocation adds or updates a user-location association.
func (s *Service) AddUserLocation(
	ctx context.Context,
	req *connect.Request[api.AddUserLocationRequest],
) (*connect.Response[api.AddUserLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"location_id", req.Msg.LocationId,
	)

	logger.DebugContext(ctx, "adding user location")

	// Check if association already exists
	existing, err := s.storage.QueryByFields(
		ctx,
		map[string]any{
			"user_id":     authInfo.UserID,
			"location_id": req.Msg.LocationId,
		},
		&models.UserLocation{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to check existing association", "error", err)
		return nil, connecterr.Internal(ctx, "AddUserLocation", fmt.Errorf("failed to check existing association"))
	}

	now := time.Now().Unix()

	if len(existing) > 0 {
		// Update existing association (refresh last_used_at)
		userLoc := existing[0].(*models.UserLocation)
		userLoc.LastUsedAtUnixSec = now

		err = s.storage.Update(ctx, userLoc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to update user location", "error", err)
			return nil, connecterr.Internal(ctx, "AddUserLocation", fmt.Errorf("failed to update user location"))
		}

		logger.InfoContext(ctx, "updated existing user location")
	} else {
		// Create new association
		userLoc := &models.UserLocation{
			Id:                uuid.NewString(),
			UserId:            authInfo.UserID,
			LocationId:        req.Msg.LocationId,
			LastUsedAtUnixSec: now,
		}

		_, err = s.storage.Insert(ctx, userLoc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to insert user location", "error", err)
			return nil, connecterr.Internal(ctx, "AddUserLocation", fmt.Errorf("failed to add user location"))
		}

		logger.InfoContext(ctx, "created new user location")
	}

	return connect.NewResponse(&api.AddUserLocationResponse{}), nil
}

// RemoveUserLocation deletes a user-location association.
func (s *Service) RemoveUserLocation(
	ctx context.Context,
	req *connect.Request[api.RemoveUserLocationRequest],
) (*connect.Response[api.RemoveUserLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"location_id", req.Msg.LocationId,
	)

	logger.InfoContext(ctx, "removing user location")

	// Find the association
	existing, err := s.storage.QueryByFields(
		ctx,
		map[string]any{
			"user_id":     authInfo.UserID,
			"location_id": req.Msg.LocationId,
		},
		&models.UserLocation{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to find user location", "error", err)
		return nil, connecterr.Internal(ctx, "RemoveUserLocation", fmt.Errorf("failed to find user location"))
	}

	if len(existing) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user location not found"))
	}

	userLoc := existing[0].(*models.UserLocation)

	// Hard delete
	err = s.storage.Delete(ctx, userLoc)
	if err != nil {
		logger.ErrorContext(ctx, "failed to delete user location", "error", err)
		return nil, connecterr.Internal(ctx, "RemoveUserLocation", fmt.Errorf("failed to remove user location"))
	}

	logger.InfoContext(ctx, "deleted user location")

	return connect.NewResponse(&api.RemoveUserLocationResponse{}), nil
}

// UpdateLocationLastUsed updates the last_used_at timestamp for a location.
func (s *Service) UpdateLocationLastUsed(
	ctx context.Context,
	req *connect.Request[api.UpdateLocationLastUsedRequest],
) (*connect.Response[api.UpdateLocationLastUsedResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"location_id", req.Msg.LocationId,
	)

	logger.DebugContext(ctx, "updating location last used timestamp")

	// Find the association
	existing, err := s.storage.QueryByFields(
		ctx,
		map[string]any{
			"user_id":     authInfo.UserID,
			"location_id": req.Msg.LocationId,
		},
		&models.UserLocation{},
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to find user location", "error", err)
		return nil, connecterr.Internal(ctx, "UpdateLocationLastUsed", fmt.Errorf("failed to find user location"))
	}

	if len(existing) == 0 {
		// Association doesn't exist yet - create it
		userLoc := &models.UserLocation{
			Id:                uuid.NewString(),
			UserId:            authInfo.UserID,
			LocationId:        req.Msg.LocationId,
			LastUsedAtUnixSec: time.Now().Unix(),
		}

		_, err = s.storage.Insert(ctx, userLoc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to create user location", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateLocationLastUsed", fmt.Errorf("failed to update location timestamp"))
		}

		logger.InfoContext(ctx, "created new user location with timestamp")
	} else {
		// Update existing association
		userLoc := existing[0].(*models.UserLocation)
		userLoc.LastUsedAtUnixSec = time.Now().Unix()

		err = s.storage.Update(ctx, userLoc)
		if err != nil {
			logger.ErrorContext(ctx, "failed to update timestamp", "error", err)
			return nil, connecterr.Internal(ctx, "UpdateLocationLastUsed", fmt.Errorf("failed to update location timestamp"))
		}

		logger.DebugContext(ctx, "updated location last used timestamp")
	}

	return connect.NewResponse(&api.UpdateLocationLastUsedResponse{}), nil
}

// ensureLocationInUserList ensures a location is in the user's UserLocation table.
// Creates a new entry if it doesn't exist, or updates last_used_at if it does.
func (s *Service) ensureLocationInUserList(ctx context.Context, userID, locationID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"user_id", userID,
		"location_id", locationID,
	)

	// Create or update UserLocation entry
	existing, err := s.storage.QueryByFields(
		ctx,
		map[string]any{
			"user_id":     userID,
			"location_id": locationID,
		},
		&models.UserLocation{},
	)
	if err != nil {
		return fmt.Errorf("failed to check user location: %w", err)
	}

	now := time.Now().Unix()

	if len(existing) > 0 {
		// Update existing entry (refresh last_used_at)
		userLoc := existing[0].(*models.UserLocation)
		userLoc.LastUsedAtUnixSec = now

		err = s.storage.Update(ctx, userLoc)
		if err != nil {
			return fmt.Errorf("failed to update user location: %w", err)
		}

		logger.DebugContext(ctx, "updated user location last_used_at")
	} else {
		// Create new entry
		userLoc := &models.UserLocation{
			Id:                uuid.NewString(),
			UserId:            userID,
			LocationId:        locationID,
			LastUsedAtUnixSec: now,
		}

		_, err = s.storage.Insert(ctx, userLoc)
		if err != nil {
			return fmt.Errorf("failed to insert user location: %w", err)
		}

		logger.InfoContext(ctx, "created new user location entry")
	}

	return nil
}
