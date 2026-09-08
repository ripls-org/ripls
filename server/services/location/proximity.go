package location

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

const (
	maxNearbyRadiusMeters     = 50_000.0
	maxNearbyResults          = 50
	nearbyCoordinatePrecision = 1000.0 // multiply then divide to get 3-decimal quantization (~110 m)
)

// quantizeCoordinate rounds a coordinate to 3 decimal places (~110 m at the equator).
func quantizeCoordinate(x float64) float64 {
	return math.Round(x*nearbyCoordinatePrecision) / nearbyCoordinatePrecision
}

// nearbySpec is the entity-specific half of a proximity search: which stored
// message to search and which of its fields carry the location reference and
// the owner.
type nearbySpec struct {
	prototype        proto.Message
	locationRefField string
	ownerField       string
	// plural names the entity in log lines ("users", "gear").
	plural string
}

// nearbySearch runs the half of GetNearbyUsers / GetNearbyGear that does not
// depend on the entity: require auth, validate the request, require active
// community membership, run the spatial query, and cap the result count.
//
// The two RPCs were 70 duplicated lines before this was extracted (#2816); what
// genuinely differs is only the response mapping, which stays with each caller.
func (s *Service) nearbySearch(
	ctx context.Context, operation string,
	communityID string, latDeg, lonDeg, radiusMeters float64,
	spec nearbySpec,
) ([]storage.SpatialQueryResult, error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", operation,
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"community_id", communityID,
	)

	if communityID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}
	if radiusMeters <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("radius_meters must be > 0"))
	}
	if radiusMeters > maxNearbyRadiusMeters {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("radius_meters must be ≤ %.0f", maxNearbyRadiusMeters))
	}

	if _, _, memberErr := auth.RequireMemberOfActiveCommunity(ctx, s.storage, communityID, authInfo.UserID); memberErr != nil {
		return nil, memberErr
	}

	logger.DebugContext(
		ctx, "searching for nearby "+spec.plural,
		"latitude_deg", latDeg,
		"longitude_deg", lonDeg,
		"radius_meters", radiusMeters,
	)

	results, err := s.storage.QueryByProximityForCommunity(
		ctx, latDeg, lonDeg, radiusMeters, communityID,
		spec.prototype, spec.locationRefField, spec.ownerField,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to query nearby "+spec.plural, "error", err)
		return nil, connecterr.Internal(ctx, operation, err)
	}

	if len(results) > maxNearbyResults {
		results = results[:maxNearbyResults]
	}
	return results, nil
}

// GetNearbyUsers finds community members within a specified radius of a query point,
// returning quantized coordinates and enforcing community membership on both caller and result.
func (s *Service) GetNearbyUsers(
	ctx context.Context,
	req *connect.Request[api.GetNearbyUsersRequest],
) (*connect.Response[api.GetNearbyUsersResponse], error) {
	results, err := s.nearbySearch(ctx, "GetNearbyUsers",
		req.Msg.CommunityId, req.Msg.LatitudeDeg, req.Msg.LongitudeDeg, req.Msg.RadiusMeters,
		nearbySpec{
			prototype:        &models.User{},
			locationRefField: "primary_residence_location_id",
			ownerField:       "id",
			plural:           "users",
		})
	if err != nil {
		return nil, err
	}

	nearbyUsers := make([]*api.NearbyUser, 0, len(results))
	for _, result := range results {
		user := result.Message.(*models.User)
		nearbyUsers = append(nearbyUsers, &api.NearbyUser{
			UserId:         user.Id,
			DistanceMeters: result.DistanceMeters,
			LatitudeDeg:    quantizeCoordinate(result.LatitudeDeg),
			LongitudeDeg:   quantizeCoordinate(result.LongitudeDeg),
		})
	}

	return connect.NewResponse(&api.GetNearbyUsersResponse{Users: nearbyUsers}), nil
}

// GetNearbyGear finds community-member-owned gear within a specified radius of a query point,
// returning quantized coordinates and enforcing community membership on both caller and gear owner.
func (s *Service) GetNearbyGear(
	ctx context.Context,
	req *connect.Request[api.GetNearbyGearRequest],
) (*connect.Response[api.GetNearbyGearResponse], error) {
	results, err := s.nearbySearch(ctx, "GetNearbyGear",
		req.Msg.CommunityId, req.Msg.LatitudeDeg, req.Msg.LongitudeDeg, req.Msg.RadiusMeters,
		nearbySpec{
			prototype:        &models.Gear{},
			locationRefField: "location_id",
			ownerField:       "owner_id",
			plural:           "gear",
		})
	if err != nil {
		return nil, err
	}

	nearbyGear := make([]*api.NearbyGear, 0, len(results))
	for _, result := range results {
		gear := result.Message.(*models.Gear)
		nearbyGear = append(nearbyGear, &api.NearbyGear{
			GearId:         gear.Id,
			DistanceMeters: result.DistanceMeters,
			LatitudeDeg:    quantizeCoordinate(result.LatitudeDeg),
			LongitudeDeg:   quantizeCoordinate(result.LongitudeDeg),
		})
	}

	return connect.NewResponse(&api.GetNearbyGearResponse{Gear: nearbyGear}), nil
}
