// Geospatial proximity queries: QueryByProximity and QueryByProximityForCommunity.

package storage

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// QueryByProximity performs a geospatial proximity query, returning messages within
// a specified radius of a center point, sorted by distance.
// The msgType parameter should be an empty instance of the desired message type.
// The message type must have latitude_deg and longitude_deg fields.
func (s *ProtoSQLStorage) QueryByProximity(
	ctx context.Context,
	centerLatDeg, centerLonDeg, radiusMeters float64,
	msgType proto.Message,
) ([]SpatialQueryResult, error) {
	descriptor := msgType.ProtoReflect().Descriptor()
	typeName := string(descriptor.FullName())

	// Verify this type is allowed for storage
	tableName, ok := s.allowedTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("message type %s is not registered for storage", typeName)
	}

	// Verify the message has geospatial fields
	hasLatLon, latField, lonField := detectGeospatialFields(descriptor)
	if !hasLatLon {
		return nil, fmt.Errorf("message type %s does not have latitude_deg and longitude_deg fields", typeName)
	}

	// Calculate bounding box for efficient filtering
	bbox := CalculateBoundingBox(centerLatDeg, centerLonDeg, radiusMeters)

	quotedTable, err := quoteIdent(tableName)
	if err != nil {
		return nil, err
	}
	quotedLat, err := quoteIdent(latField)
	if err != nil {
		return nil, err
	}
	quotedLon, err := quoteIdent(lonField)
	if err != nil {
		return nil, err
	}

	// Query with bounding box filter
	query := fmt.Sprintf(
		"SELECT binary_proto, %s, %s FROM %s WHERE %s BETWEEN %s AND %s AND %s BETWEEN %s AND %s",
		quotedLat, quotedLon, quotedTable,
		quotedLat, s.dbSpec.Placeholder(1), s.dbSpec.Placeholder(2),
		quotedLon, s.dbSpec.Placeholder(3), s.dbSpec.Placeholder(4),
	)

	rows, err := s.db.QueryContext(
		ctx, query,
		bbox.MinLatDeg, bbox.MaxLatDeg,
		bbox.MinLonDeg, bbox.MaxLonDeg,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query proximity from %s: %w", tableName, err)
	}
	defer rows.Close()

	var results []SpatialQueryResult
	for rows.Next() {
		var protoData []byte
		var lat, lon float64

		if err := rows.Scan(&protoData, &lat, &lon); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Calculate precise distance using Haversine formula
		distance := HaversineDistance(centerLatDeg, centerLonDeg, lat, lon)

		// Filter by exact radius (bounding box may include points outside the circle)
		if distance <= radiusMeters {
			// Create a new instance of the message type
			msg := proto.Clone(msgType)
			proto.Reset(msg)

			if err := proto.Unmarshal(protoData, msg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal proto: %w", err)
			}

			results = append(results, SpatialQueryResult{
				Message:        msg,
				DistanceMeters: distance,
				LatitudeDeg:    lat,
				LongitudeDeg:   lon,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Sort results by distance (ascending)
	// We use a simple bubble sort since the number of results is typically small
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].DistanceMeters > results[j].DistanceMeters {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// QueryByProximityForCommunity performs a community-scoped geospatial proximity query by
// joining with a location table and filtering to entities whose owners are active members
// of communityID.
//
// ownerField is the column on the entity table that holds the user ID checked against
// community membership. For models.User pass "id" (a User is its own owner). For
// models.Gear pass "owner_id".
//
// Soft-deleted entity rows and soft-deleted community memberships are excluded. Results
// are sorted by distance ascending; the caller applies any result cap.
func (s *ProtoSQLStorage) QueryByProximityForCommunity(
	ctx context.Context,
	centerLatDeg, centerLonDeg, radiusMeters float64,
	communityID string,
	entityMsgType proto.Message,
	locationRefField, ownerField string,
) ([]SpatialQueryResult, error) {
	entityDescriptor := entityMsgType.ProtoReflect().Descriptor()
	entityTypeName := string(entityDescriptor.FullName())

	entityTableName, err := s.quotedTableFor(entityTypeName)
	if err != nil {
		return nil, err
	}

	locationTableName, err := s.quotedTableFor("ripls.models.Location")
	if err != nil {
		return nil, err
	}

	cuTableName, err := s.quotedTableFor("ripls.models.CommunityUser")
	if err != nil {
		return nil, err
	}

	// locationRefField and ownerField are supplied by the caller. Every caller
	// passes a literal today, but nothing enforced that before #2795 — this is
	// the one public storage entry point that took an identifier straight from
	// its argument list into the query text.
	quotedLocationRef, err := qualify("e", locationRefField)
	if err != nil {
		return nil, err
	}
	quotedOwner, err := qualify("e", ownerField)
	if err != nil {
		return nil, err
	}

	locationDescriptor := (&models.Location{}).ProtoReflect().Descriptor()
	hasLatLon, latField, lonField := detectGeospatialFields(locationDescriptor)
	if !hasLatLon {
		return nil, fmt.Errorf("location type does not have latitude_deg and longitude_deg fields")
	}

	bbox := CalculateBoundingBox(centerLatDeg, centerLonDeg, radiusMeters)

	locLat, err := qualify("l", latField)
	if err != nil {
		return nil, err
	}
	locLon, err := qualify("l", lonField)
	if err != nil {
		return nil, err
	}
	entityDeleted, err := qualify("e", deletedFilterColumn)
	if err != nil {
		return nil, err
	}
	cuDeleted, err := qualify("cu", deletedFilterColumn)
	if err != nil {
		return nil, err
	}

	p := s.dbSpec.Placeholder
	query := fmt.Sprintf(
		`SELECT e.binary_proto, %s, %s
		FROM %s e
		INNER JOIN %s l ON %s = l.id
		INNER JOIN %s cu ON cu.user_id = %s AND cu.community_id = %s
		WHERE COALESCE(%s, 0) = 0
		AND COALESCE(%s, 0) = 0
		AND %s BETWEEN %s AND %s
		AND %s BETWEEN %s AND %s`,
		locLat, locLon,
		entityTableName,
		locationTableName, quotedLocationRef,
		cuTableName, quotedOwner, p(5),
		entityDeleted,
		cuDeleted,
		locLat, p(1), p(2),
		locLon, p(3), p(4),
	)

	rows, err := s.db.QueryContext(
		ctx, query,
		bbox.MinLatDeg, bbox.MaxLatDeg,
		bbox.MinLonDeg, bbox.MaxLonDeg,
		communityID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query proximity for community: %w", err)
	}
	defer rows.Close()

	var results []SpatialQueryResult
	for rows.Next() {
		var protoData []byte
		var lat, lon float64

		if err := rows.Scan(&protoData, &lat, &lon); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		distance := HaversineDistance(centerLatDeg, centerLonDeg, lat, lon)

		if distance <= radiusMeters {
			msg := proto.Clone(entityMsgType)
			proto.Reset(msg)

			if err := proto.Unmarshal(protoData, msg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal proto: %w", err)
			}

			results = append(results, SpatialQueryResult{
				Message:        msg,
				DistanceMeters: distance,
				LatitudeDeg:    lat,
				LongitudeDeg:   lon,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DistanceMeters < results[j].DistanceMeters
	})

	return results, nil
}
