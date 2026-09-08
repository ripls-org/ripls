package storage

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// insertTestLocation is a helper that inserts a Location at the given lat/lng and returns its ID.
func insertTestLocation(t *testing.T, s *ProtoSQLStorage, lat, lon float64) string {
	t.Helper()
	loc := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  lat,
			LongitudeDeg: lon,
		},
		Address: &models.Address{Locality: "Test"},
	}
	id, err := s.Insert(context.Background(), loc)
	if err != nil {
		t.Fatalf("insertTestLocation: %v", err)
	}
	return id
}

// insertTestCommunityUser inserts a CommunityUser membership and returns the row.
func insertTestCommunityUser(t *testing.T, s *ProtoSQLStorage, communityID, userID string) *models.CommunityUser {
	t.Helper()
	cu := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      userID,
	}
	if _, err := s.Insert(context.Background(), cu); err != nil {
		t.Fatalf("insertTestCommunityUser: %v", err)
	}
	return cu
}

func TestQueryByProximityForCommunity_Users(t *testing.T) {
	// Austin, TX area coordinates used throughout.
	const (
		austinLat = 30.2672
		austinLon = -97.7431
	)

	communityID := uuid.New().String()

	t.Run("returns member whose location is in range", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		locID := insertTestLocation(t, s, austinLat, austinLon)

		userID := "user-member-in-range"
		user := &models.User{
			Id:                         userID,
			Email:                      "member@example.com",
			PrimaryResidenceLocationId: locID,
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		insertTestCommunityUser(t, s, communityID, userID)

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			communityID, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		got := results[0].Message.(*models.User)
		if got.Id != userID {
			t.Errorf("expected user %s, got %s", userID, got.Id)
		}
	})

	t.Run("excludes user not in community", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		locID := insertTestLocation(t, s, austinLat, austinLon)

		nonMemberID := "user-nonmember"
		user := &models.User{
			Id:                         nonMemberID,
			Email:                      "nonmember@example.com",
			PrimaryResidenceLocationId: locID,
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		// No CommunityUser row inserted.

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			communityID, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for non-member, got %d", len(results))
		}
	})

	t.Run("excludes soft-deleted community membership", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		locID := insertTestLocation(t, s, austinLat, austinLon)

		userID := "user-deleted-member"
		user := &models.User{
			Id:                         userID,
			Email:                      "deleted-member@example.com",
			PrimaryResidenceLocationId: locID,
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		cu := insertTestCommunityUser(t, s, communityID, userID)
		cu.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1000000}
		if err := s.Update(ctx, cu); err != nil {
			t.Fatalf("soft-delete membership: %v", err)
		}

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			communityID, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for soft-deleted membership, got %d", len(results))
		}
	})

	t.Run("excludes soft-deleted user entity", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		locID := insertTestLocation(t, s, austinLat, austinLon)

		userID := "user-deleted-entity"
		user := &models.User{
			Id:                         userID,
			Email:                      "deleted@example.com",
			PrimaryResidenceLocationId: locID,
			Deleted:                    &models.DeletedMetadata{DeletedAtUnixSec: 1000000},
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert deleted user: %v", err)
		}
		insertTestCommunityUser(t, s, communityID, userID)

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			communityID, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for soft-deleted user, got %d", len(results))
		}
	})

	t.Run("bounding box and haversine filter exclude out-of-radius member", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		// Dallas ~300 km from Austin.
		dallasLat, dallasLon := 32.7767, -96.7970
		locID := insertTestLocation(t, s, dallasLat, dallasLon)

		userID := "user-dallas-member"
		user := &models.User{
			Id:                         userID,
			Email:                      "dallas@example.com",
			PrimaryResidenceLocationId: locID,
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		insertTestCommunityUser(t, s, communityID, userID)

		// 50 km radius from Austin should not reach Dallas.
		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			communityID, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results (out of radius), got %d", len(results))
		}
	})

	t.Run("returns empty slice for unknown communityID", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		locID := insertTestLocation(t, s, austinLat, austinLon)
		userID := "user-any"
		user := &models.User{
			Id:                         userID,
			Email:                      "any@example.com",
			PrimaryResidenceLocationId: locID,
		}
		if _, err := s.Insert(ctx, user); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		insertTestCommunityUser(t, s, communityID, userID)

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			"unknown-community-id", &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for unknown community, got %d", len(results))
		}
	})

	t.Run("sorts results by distance ascending", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		cid := uuid.New().String()

		// Two users at increasing distance from query point.
		loc1 := insertTestLocation(t, s, 30.2672, -97.7431) // ~0 m
		loc2 := insertTestLocation(t, s, 30.2772, -97.7431) // ~1.1 km
		loc3 := insertTestLocation(t, s, 30.2872, -97.7431) // ~2.2 km

		for i, id := range []string{"u1", "u2", "u3"} {
			locs := []string{loc1, loc2, loc3}
			u := &models.User{
				Id:                         id,
				Email:                      id + "@example.com",
				PrimaryResidenceLocationId: locs[i],
			}
			if _, err := s.Insert(ctx, u); err != nil {
				t.Fatalf("insert user %s: %v", id, err)
			}
			insertTestCommunityUser(t, s, cid, id)
		}

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			cid, &models.User{},
			"primary_residence_location_id", "id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		for i := 0; i < len(results)-1; i++ {
			if results[i].DistanceMeters > results[i+1].DistanceMeters {
				t.Errorf("results not sorted by distance: result[%d]=%f > result[%d]=%f",
					i, results[i].DistanceMeters, i+1, results[i+1].DistanceMeters)
			}
		}
	})
}

func TestQueryByProximityForCommunity_Gear(t *testing.T) {
	const (
		austinLat = 30.2672
		austinLon = -97.7431
	)

	t.Run("returns gear owned by community member", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		cid := uuid.New().String()
		ownerID := "gear-owner"
		insertTestCommunityUser(t, s, cid, ownerID)

		locID := insertTestLocation(t, s, austinLat, austinLon)
		gear := &models.Gear{
			Id:         "gear-1",
			OwnerId:    ownerID,
			LocationId: locID,
		}
		if _, err := s.Insert(ctx, gear); err != nil {
			t.Fatalf("insert gear: %v", err)
		}

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			cid, &models.Gear{},
			"location_id", "owner_id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("excludes gear owned by non-member", func(t *testing.T) {
		ctx := context.Background()
		s, cleanup := SetupTestStorage(t)
		t.Cleanup(cleanup)

		cid := uuid.New().String()
		nonMemberID := "nonmember-owner"
		// No community membership row.

		locID := insertTestLocation(t, s, austinLat, austinLon)
		gear := &models.Gear{
			Id:         "gear-2",
			OwnerId:    nonMemberID,
			LocationId: locID,
		}
		if _, err := s.Insert(ctx, gear); err != nil {
			t.Fatalf("insert gear: %v", err)
		}

		results, err := s.QueryByProximityForCommunity(
			ctx,
			austinLat, austinLon, 50000,
			cid, &models.Gear{},
			"location_id", "owner_id",
		)
		if err != nil {
			t.Fatalf("QueryByProximityForCommunity: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results for non-member gear owner, got %d", len(results))
		}
	})
}
