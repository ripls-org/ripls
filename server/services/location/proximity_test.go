package location

import (
	"context"
	"fmt"
	"math"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// Austin, TX is the canonical test center.
const (
	austinLat = 30.2672
	austinLon = -97.7431
)

// testFixture holds a Service with a backing storage, a community, and a member caller.
type testFixture struct {
	service     *Service
	storage     *storage.ProtoSQLStorage
	communityID string
	callerID    string
}

// authCtx returns an authenticated context for the fixture's caller.
func (f *testFixture) authCtx() context.Context {
	return createAuthContext(f.callerID, f.callerID+"@example.com")
}

// newFixture creates a Service, inserts a community, and enrolls a caller as a member.
func newFixture(t *testing.T) *testFixture {
	t.Helper()
	svc, sqlStorage, _ := setupTestService(t)

	communityID := uuid.New().String()
	callerID := uuid.New().String()

	if _, err := sqlStorage.Insert(context.Background(), &models.Community{
		Id:          communityID,
		Name:        "test-community",
		CreatorId:   callerID,
		OwnerUserId: callerID,
	}); err != nil {
		t.Fatalf("newFixture: insert community: %v", err)
	}
	if _, err := sqlStorage.Insert(context.Background(), &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      callerID,
	}); err != nil {
		t.Fatalf("newFixture: insert caller membership: %v", err)
	}

	return &testFixture{
		service:     svc,
		storage:     sqlStorage,
		communityID: communityID,
		callerID:    callerID,
	}
}

// insertLoc inserts a Location at lat/lon and returns its ID.
func (f *testFixture) insertLoc(t *testing.T, lat, lon float64) string {
	t.Helper()
	id, err := f.storage.Insert(context.Background(), &models.Location{
		Geolocation: &models.Geolocation{LatitudeDeg: lat, LongitudeDeg: lon},
		Address:     &models.Address{Locality: "Test"},
	})
	if err != nil {
		t.Fatalf("insertLoc: %v", err)
	}
	return id
}

// addMember inserts a User with the given locationID and enrolls it in f.communityID.
func (f *testFixture) addMember(t *testing.T, locID string) *models.User {
	t.Helper()
	userID := uuid.New().String()
	u := &models.User{
		Id:                         userID,
		Email:                      userID + "@example.com",
		PrimaryResidenceLocationId: locID,
	}
	if _, err := f.storage.Insert(context.Background(), u); err != nil {
		t.Fatalf("addMember: insert user: %v", err)
	}
	if _, err := f.storage.Insert(context.Background(), &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: f.communityID,
		UserId:      userID,
	}); err != nil {
		t.Fatalf("addMember: insert membership: %v", err)
	}
	return u
}

// addGearForMember inserts Gear owned by ownerID at locID.
func (f *testFixture) addGearForMember(t *testing.T, ownerID, locID string) *models.Gear {
	t.Helper()
	g := &models.Gear{
		Id:         uuid.New().String(),
		OwnerId:    ownerID,
		LocationId: locID,
	}
	if _, err := f.storage.Insert(context.Background(), g); err != nil {
		t.Fatalf("addGearForMember: insert gear: %v", err)
	}
	return g
}

// insertMsg is a convenience wrapper for inserting any proto.Message.
func (f *testFixture) insertMsg(t *testing.T, msg proto.Message) string {
	t.Helper()
	id, err := f.storage.Insert(context.Background(), msg)
	if err != nil {
		t.Fatalf("insertMsg: %v", err)
	}
	return id
}

func TestService_GetNearbyUsers(t *testing.T) {
	t.Run("returns nearby member sorted by distance", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)
		member := f.addMember(t, locID)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(resp.Msg.Users))
		}
		if resp.Msg.Users[0].UserId != member.Id {
			t.Errorf("expected user %s, got %s", member.Id, resp.Msg.Users[0].UserId)
		}
		if resp.Msg.Users[0].DistanceMeters > 1000 {
			t.Errorf("expected distance < 1000 m, got %f", resp.Msg.Users[0].DistanceMeters)
		}
	})

	t.Run("multiple members sorted by distance ascending", func(t *testing.T) {
		f := newFixture(t)
		loc1 := f.insertLoc(t, austinLat, austinLon)
		loc2 := f.insertLoc(t, 30.2772, austinLon) // ~1.1 km north
		f.addMember(t, loc1)
		f.addMember(t, loc2)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 5000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 2 {
			t.Fatalf("expected 2 users, got %d", len(resp.Msg.Users))
		}
		if resp.Msg.Users[0].DistanceMeters > resp.Msg.Users[1].DistanceMeters {
			t.Error("expected results sorted by distance ascending")
		}
	})

	t.Run("no members within radius returns empty", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  0.0,
			LongitudeDeg: 0.0,
			RadiusMeters: 1000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 0 {
			t.Errorf("expected 0 users, got %d", len(resp.Msg.Users))
		}
	})

	t.Run("unauthenticated request returns Unauthenticated", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyUsers(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for unauthenticated request")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("expected Unauthenticated, got %v", connectErr.Code())
		}
	})

	t.Run("empty community_id returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  "",
		})
		_, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for empty community_id")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("radius == 0 returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 0,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for radius == 0")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("radius above cap returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: maxNearbyRadiusMeters + 1,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for radius above cap")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("non-member caller returns PermissionDenied", func(t *testing.T) {
		f := newFixture(t)
		nonMemberCtx := createAuthContext(uuid.New().String(), "nonmember@example.com")

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyUsers(nonMemberCtx, req)
		if err == nil {
			t.Fatal("expected error for non-member caller")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("non-member target user is excluded", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)

		// User in range but with no community membership row.
		nonMemberUser := &models.User{
			Id:                         uuid.New().String(),
			Email:                      "target-nonmember@example.com",
			PrimaryResidenceLocationId: locID,
		}
		f.insertMsg(t, nonMemberUser)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 0 {
			t.Errorf("expected non-member target to be excluded, got %d users", len(resp.Msg.Users))
		}
	})

	t.Run("quantization rounds coordinates to 3 decimal places", func(t *testing.T) {
		f := newFixture(t)
		rawLat, rawLon := 30.26723456, -97.74318765
		locID := f.insertLoc(t, rawLat, rawLon)
		f.addMember(t, locID)

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 1 {
			t.Fatalf("expected 1 user, got %d", len(resp.Msg.Users))
		}
		wantLat := math.Round(rawLat*1000) / 1000
		wantLon := math.Round(rawLon*1000) / 1000
		if resp.Msg.Users[0].LatitudeDeg != wantLat {
			t.Errorf("quantized lat: got %f, want %f", resp.Msg.Users[0].LatitudeDeg, wantLat)
		}
		if resp.Msg.Users[0].LongitudeDeg != wantLon {
			t.Errorf("quantized lon: got %f, want %f", resp.Msg.Users[0].LongitudeDeg, wantLon)
		}
	})

	t.Run("result cap returns at most maxNearbyResults", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)
		for i := 0; i < 60; i++ {
			f.addMember(t, locID)
		}

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) > maxNearbyResults {
			t.Errorf("expected at most %d results, got %d", maxNearbyResults, len(resp.Msg.Users))
		}
	})

	t.Run("soft-deleted target user is excluded", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)

		deletedUserID := uuid.New().String()
		f.insertMsg(t, &models.User{
			Id:                         deletedUserID,
			Email:                      "deleted@example.com",
			PrimaryResidenceLocationId: locID,
			Deleted:                    &models.DeletedMetadata{DeletedAtUnixSec: 1000000},
		})
		f.insertMsg(t, &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: f.communityID,
			UserId:      deletedUserID,
		})

		req := connect.NewRequest(&api.GetNearbyUsersRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyUsers(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyUsers: %v", err)
		}
		if len(resp.Msg.Users) != 0 {
			t.Errorf("expected deleted user to be excluded, got %d users", len(resp.Msg.Users))
		}
	})
}

func TestService_GetNearbyGear(t *testing.T) {
	t.Run("returns gear owned by community member", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)
		owner := f.addMember(t, locID)
		gear := f.addGearForMember(t, owner.Id, locID)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) != 1 {
			t.Fatalf("expected 1 gear, got %d", len(resp.Msg.Gear))
		}
		if resp.Msg.Gear[0].GearId != gear.Id {
			t.Errorf("expected gear %s, got %s", gear.Id, resp.Msg.Gear[0].GearId)
		}
	})

	t.Run("multiple gear sorted by distance ascending", func(t *testing.T) {
		f := newFixture(t)

		locs := []struct{ lat, lon float64 }{
			{austinLat, austinLon},
			{30.2772, austinLon},
			{30.2872, austinLon},
		}
		for _, loc := range locs {
			ownerLocID := f.insertLoc(t, austinLat, austinLon)
			owner := f.addMember(t, ownerLocID)
			gearLocID := f.insertLoc(t, loc.lat, loc.lon)
			f.addGearForMember(t, owner.Id, gearLocID)
		}

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 5000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) != 3 {
			t.Fatalf("expected 3 gear, got %d", len(resp.Msg.Gear))
		}
		for i := 0; i < len(resp.Msg.Gear)-1; i++ {
			if resp.Msg.Gear[i].DistanceMeters > resp.Msg.Gear[i+1].DistanceMeters {
				t.Error("expected gear sorted by distance ascending")
			}
		}
	})

	t.Run("no gear within radius returns empty", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  0.0,
			LongitudeDeg: 0.0,
			RadiusMeters: 1000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) != 0 {
			t.Errorf("expected 0 gear, got %d", len(resp.Msg.Gear))
		}
	})

	t.Run("unauthenticated request returns Unauthenticated", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyGear(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for unauthenticated request")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("expected Unauthenticated, got %v", connectErr.Code())
		}
	})

	t.Run("empty community_id returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  "",
		})
		_, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for empty community_id")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("radius == 0 returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 0,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for radius == 0")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("radius above cap returns InvalidArgument", func(t *testing.T) {
		f := newFixture(t)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: maxNearbyRadiusMeters + 1,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err == nil {
			t.Fatal("expected error for radius above cap")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", connectErr.Code())
		}
	})

	t.Run("non-member caller returns PermissionDenied", func(t *testing.T) {
		f := newFixture(t)
		nonMemberCtx := createAuthContext(uuid.New().String(), "nonmember@example.com")

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 10000,
			CommunityId:  f.communityID,
		})
		_, err := f.service.GetNearbyGear(nonMemberCtx, req)
		if err == nil {
			t.Fatal("expected error for non-member caller")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("gear owned by non-member is excluded", func(t *testing.T) {
		f := newFixture(t)
		locID := f.insertLoc(t, austinLat, austinLon)

		nonMemberID := uuid.New().String()
		f.insertMsg(t, &models.Gear{
			Id:         uuid.New().String(),
			OwnerId:    nonMemberID,
			LocationId: locID,
		})

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) != 0 {
			t.Errorf("expected non-member gear to be excluded, got %d gear", len(resp.Msg.Gear))
		}
	})

	t.Run("quantization rounds coordinates to 3 decimal places", func(t *testing.T) {
		f := newFixture(t)
		rawLat, rawLon := 30.26723456, -97.74318765
		gearLocID := f.insertLoc(t, rawLat, rawLon)
		ownerLocID := f.insertLoc(t, austinLat, austinLon)
		owner := f.addMember(t, ownerLocID)
		f.addGearForMember(t, owner.Id, gearLocID)

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) != 1 {
			t.Fatalf("expected 1 gear, got %d", len(resp.Msg.Gear))
		}
		wantLat := math.Round(rawLat*1000) / 1000
		wantLon := math.Round(rawLon*1000) / 1000
		if resp.Msg.Gear[0].LatitudeDeg != wantLat {
			t.Errorf("quantized lat: got %f, want %f", resp.Msg.Gear[0].LatitudeDeg, wantLat)
		}
		if resp.Msg.Gear[0].LongitudeDeg != wantLon {
			t.Errorf("quantized lon: got %f, want %f", resp.Msg.Gear[0].LongitudeDeg, wantLon)
		}
	})

	t.Run("result cap returns at most maxNearbyResults gear", func(t *testing.T) {
		f := newFixture(t)
		for i := 0; i < 60; i++ {
			ownerLocID := f.insertLoc(t, austinLat, austinLon)
			owner := f.addMember(t, ownerLocID)
			gearLocID := f.insertLoc(t, austinLat+float64(i)*0.0001, austinLon)
			f.insertMsg(t, &models.Gear{
				Id:         fmt.Sprintf("gear-cap-%d", i),
				OwnerId:    owner.Id,
				LocationId: gearLocID,
			})
		}

		req := connect.NewRequest(&api.GetNearbyGearRequest{
			LatitudeDeg:  austinLat,
			LongitudeDeg: austinLon,
			RadiusMeters: 50000,
			CommunityId:  f.communityID,
		})
		resp, err := f.service.GetNearbyGear(f.authCtx(), req)
		if err != nil {
			t.Fatalf("GetNearbyGear: %v", err)
		}
		if len(resp.Msg.Gear) > maxNearbyResults {
			t.Errorf("expected at most %d results, got %d", maxNearbyResults, len(resp.Msg.Gear))
		}
	})
}
