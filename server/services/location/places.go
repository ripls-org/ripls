package location

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// SaveLocation creates a new location in storage and automatically adds it to the user's location list.
//
// The location is automatically managed:
//   - If the user has no primary residence: the new location becomes their primary residence
//   - If the user has a primary residence: the new location is added to their other_location_ids
//   - Duplicates are prevented: if the location is already in the user's list, it's not added again
//
// Administrative Hierarchy:
// The client should populate the neighborhood, county, and administrative_area fields
// from Mapbox Search Box API context data when available. These fields enhance location
// data for regional filtering and community discovery. See docs/ai/regions.md for details.
func (s *Service) SaveLocation(
	ctx context.Context,
	req *connect.Request[api.SaveLocationRequest],
) (*connect.Response[api.SaveLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "saving location",
		"locality", req.Msg.Locality,
		"region_code", req.Msg.RegionCode,
		"external_place_id", req.Msg.ExternalPlaceId,
		"external_place_provider", req.Msg.ExternalPlaceProvider,
	)

	// Check for existing location (deduplication)
	existingLocation, err := s.findOrCreateLocation(
		ctx,
		authInfo.UserID,
		req.Msg.ExternalPlaceId,
		req.Msg.ExternalPlaceProvider,
		req.Msg.LatitudeDeg,
		req.Msg.LongitudeDeg,
	)
	if err != nil {
		return nil, err
	}

	// If we found an existing location, ensure it's in the user's list and return it
	if existingLocation != nil {
		logger.InfoContext(ctx, "reusing existing location", "location_id", existingLocation.Id)

		err = s.ensureLocationInUserList(ctx, authInfo.UserID, existingLocation.Id)
		if err != nil {
			logger.ErrorContext(ctx, "failed to add location to user list", "error", err)
			return nil, fmt.Errorf("failed to add location to user list: %w", err)
		}

		resp := &api.SaveLocationResponse{
			Id: existingLocation.Id,
		}
		return connect.NewResponse(resp), nil
	}

	// No duplicate found - create new location
	logger.InfoContext(ctx, "creating new location")

	// Create Location proto with enhanced administrative hierarchy
	loc := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  req.Msg.LatitudeDeg,
			LongitudeDeg: req.Msg.LongitudeDeg,
		},
		Address: &models.Address{
			RegionCode:         req.Msg.RegionCode,
			PostalCode:         req.Msg.PostalCode,
			Locality:           req.Msg.Locality,
			AddressLines:       req.Msg.AddressLines,
			Neighborhood:       &req.Msg.Neighborhood,       // NEW: from Mapbox context
			County:             &req.Msg.County,             // NEW: from Mapbox context
			AdministrativeArea: &req.Msg.AdministrativeArea, // NEW: from Mapbox context
		},
		Name:                  &req.Msg.Name,
		ExternalPlaceId:       &req.Msg.ExternalPlaceId,
		ExternalPlaceProvider: &req.Msg.ExternalPlaceProvider,
		CreatedAtUnixSec:      time.Now().Unix(),
		UpdatedAtUnixSec:      time.Now().Unix(),
	}

	// Insert into storage
	id, err := s.storage.Insert(ctx, loc)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert location", "error", err)
		return nil, connecterr.Internal(ctx, "SaveLocation", err)
	}

	logger.InfoContext(ctx, "location created", "location_id", id)

	// Automatically add location to user's list (creates/updates UserLocation entry)
	err = s.ensureLocationInUserList(ctx, authInfo.UserID, id)
	if err != nil {
		logger.ErrorContext(ctx, "failed to add location to user list", "error", err)
		return nil, fmt.Errorf("failed to add location to user list: %w", err)
	}

	resp := &api.SaveLocationResponse{
		Id: id,
	}

	return connect.NewResponse(resp), nil
}

// GetLocation retrieves a location by ID.
func (s *Service) GetLocation(
	ctx context.Context,
	req *connect.Request[api.GetLocationRequest],
) (*connect.Response[api.GetLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"location_id", req.Msg.Id,
	)

	logger.DebugContext(ctx, "getting location")

	// Fetch location from storage
	loc := &models.Location{}
	err = s.storage.GetByID(ctx, req.Msg.Id, loc)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get location", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	resp := &api.GetLocationResponse{
		Id:                    loc.Id,
		LatitudeDeg:           loc.Geolocation.LatitudeDeg,
		LongitudeDeg:          loc.Geolocation.LongitudeDeg,
		RegionCode:            loc.Address.RegionCode,
		PostalCode:            loc.Address.PostalCode,
		Locality:              loc.Address.Locality,
		AddressLines:          loc.Address.AddressLines,
		Name:                  loc.GetName(),
		Neighborhood:          loc.Address.GetNeighborhood(),
		County:                loc.Address.GetCounty(),
		AdministrativeArea:    loc.Address.GetAdministrativeArea(),
		ExternalPlaceId:       loc.GetExternalPlaceId(),
		ExternalPlaceProvider: loc.GetExternalPlaceProvider(),
	}

	return connect.NewResponse(resp), nil
}

// DeleteLocation removes a location from storage.
func (s *Service) DeleteLocation(
	ctx context.Context,
	req *connect.Request[api.DeleteLocationRequest],
) (*connect.Response[api.DeleteLocationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"location_id", req.Msg.Id,
	)

	logger.InfoContext(ctx, "deleting location")

	// Fetch the location to verify it exists
	loc := &models.Location{}
	err = s.storage.GetByID(ctx, req.Msg.Id, loc)
	if err != nil {
		logger.ErrorContext(ctx, "failed to get location", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("location not found"))
	}

	// Check if this location is referenced by any user
	// Query all users to see if any have this location as primary or in other_location_ids
	users, err := s.storage.ListAll(ctx, &models.User{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query users", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteLocation", fmt.Errorf("failed to check location references"))
	}

	// Check each user for references to this location
	for _, u := range users {
		userProto := u.(*models.User)

		// Only allow the user who owns the location to delete it
		// A location is "owned" by a user if it's in their primary or other locations
		isOwner := false
		if userProto.PrimaryResidenceLocationId == req.Msg.Id {
			isOwner = true
		}
		for _, locID := range userProto.OtherLocationIds {
			if locID == req.Msg.Id {
				isOwner = true
				break
			}
		}

		// If this user owns the location, verify it's the authenticated user
		if isOwner && userProto.Id != authInfo.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("not authorized to delete this location"))
		}

		// If the authenticated user owns it, remove references before deleting
		if isOwner && userProto.Id == authInfo.UserID {
			// Remove from primary residence if set
			if userProto.PrimaryResidenceLocationId == req.Msg.Id {
				userProto.PrimaryResidenceLocationId = ""
			}

			// Remove from other locations
			filteredLocations := []string{}
			for _, locID := range userProto.OtherLocationIds {
				if locID != req.Msg.Id {
					filteredLocations = append(filteredLocations, locID)
				}
			}
			userProto.OtherLocationIds = filteredLocations

			// Update the user
			err = s.storage.Update(ctx, userProto)
			if err != nil {
				logger.ErrorContext(ctx, "failed to update user after removing location reference", "error", err)
				return nil, connecterr.Internal(ctx, "DeleteLocation", fmt.Errorf("failed to update user references"))
			}
		}
	}

	// Check if this location is referenced by any gear
	gearItems, err := s.storage.ListAll(ctx, &models.Gear{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to query gear", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteLocation", fmt.Errorf("failed to check gear references"))
	}

	// Remove references from gear owned by the authenticated user
	for _, g := range gearItems {
		gearProto := g.(*models.Gear)
		if gearProto.LocationId == req.Msg.Id {
			// Only allow deletion if the user owns the gear
			if gearProto.OwnerId != authInfo.UserID {
				return nil, connect.NewError(connect.CodePermissionDenied,
					fmt.Errorf("location is referenced by gear you don't own"))
			}

			// Clear the location reference
			gearProto.LocationId = ""
			err = s.storage.Update(ctx, gearProto)
			if err != nil {
				logger.ErrorContext(ctx, "failed to update gear after removing location reference",
					"gear_id", gearProto.Id,
					"error", err,
				)
				return nil, connecterr.Internal(ctx, "DeleteLocation", fmt.Errorf("failed to update gear references"))
			}
		}
	}

	// Set the deleted metadata (soft delete)
	loc.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	err = s.storage.Update(ctx, loc)
	if err != nil {
		logger.ErrorContext(ctx, "failed to soft-delete location", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteLocation", fmt.Errorf("failed to delete location"))
	}

	return connect.NewResponse(&api.DeleteLocationResponse{}), nil
}
