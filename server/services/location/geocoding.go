package location

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GeocodeAddress converts an address to coordinates (forward geocoding).
func (s *Service) GeocodeAddress(
	ctx context.Context,
	req *connect.Request[api.GeocodeAddressRequest],
) (*connect.Response[api.GeocodeAddressResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "geocoding address",
		"locality", req.Msg.Locality,
		"region_code", req.Msg.RegionCode,
	)

	// Build Address from request
	address := &models.Address{
		RegionCode:   req.Msg.RegionCode,
		PostalCode:   req.Msg.PostalCode,
		Locality:     req.Msg.Locality,
		AddressLines: req.Msg.AddressLines,
	}

	// Forward geocode via the location provider.
	geolocation, err := s.locationProvider.ForwardGeocode(ctx, address)
	if err != nil {
		logger.ErrorContext(ctx, "forward geocoding failed", "error", err)
		return nil, connecterr.Internal(ctx, "GeocodeAddress", err, "detail", "geocoding failed")
	}

	resp := &api.GeocodeAddressResponse{
		LatitudeDeg:  geolocation.LatitudeDeg,
		LongitudeDeg: geolocation.LongitudeDeg,
	}

	return connect.NewResponse(resp), nil
}

// ReverseGeocode converts coordinates to an address.
func (s *Service) ReverseGeocode(
	ctx context.Context,
	req *connect.Request[api.ReverseGeocodeRequest],
) (*connect.Response[api.ReverseGeocodeResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.DebugContext(ctx, "reverse geocoding",
		"latitude_deg", req.Msg.LatitudeDeg,
		"longitude_deg", req.Msg.LongitudeDeg,
	)

	// Build Geolocation from request
	geolocation := &models.Geolocation{
		LatitudeDeg:  req.Msg.LatitudeDeg,
		LongitudeDeg: req.Msg.LongitudeDeg,
	}

	// Reverse geocode via the location provider.
	place, err := s.locationProvider.ReverseGeocode(ctx, geolocation)
	if err != nil {
		logger.ErrorContext(ctx, "reverse geocoding failed", "error", err)
		return nil, connecterr.Internal(ctx, "ReverseGeocode", err, "detail", "reverse geocoding failed")
	}

	addressLines := []string{}
	if place.FullAddress != "" {
		addressLines = []string{place.FullAddress}
	}

	resp := &api.ReverseGeocodeResponse{
		RegionCode:   place.RegionCode,
		PostalCode:   place.PostalCode,
		Locality:     place.Locality,
		AddressLines: addressLines,
	}

	return connect.NewResponse(resp), nil
}
