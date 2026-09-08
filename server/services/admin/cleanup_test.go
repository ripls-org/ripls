package admin

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestCleanupSimulation_RemovesSimulationData verifies that cleanup removes all
// data associated with a simulation run.
func TestCleanupSimulation_RemovesSimulationData(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)
	simID := "sim-2026-02-24-test"

	// Create simulation user.
	user := &models.User{
		Id:           uuid.New().String(),
		Name:         "Sim User",
		Email:        "sim@test.com",
		SimulationId: proto.String(simID),
	}
	if _, err := db.Insert(ctx, user); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Create simulation community.
	community := &models.Community{
		Id:           uuid.New().String(),
		Name:         "Sim Community",
		CreatorId:    user.Id,
		OwnerUserId:  user.Id,
		SimulationId: proto.String(simID),
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// Create community membership.
	membership := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: community.Id,
		UserId:      user.Id,
	}
	if _, err := db.Insert(ctx, membership); err != nil {
		t.Fatalf("Failed to insert membership: %v", err)
	}

	// Create gear owned by sim user.
	gear := &models.Gear{
		Id:      uuid.New().String(),
		Name:    "Sim Drill",
		OwnerId: user.Id,
	}
	if _, err := db.Insert(ctx, gear); err != nil {
		t.Fatalf("Failed to insert gear: %v", err)
	}

	// Create community gear.
	communityGear := &models.CommunityGear{
		Id:          uuid.New().String(),
		CommunityId: community.Id,
		GearId:      gear.Id,
	}
	if _, err := db.Insert(ctx, communityGear); err != nil {
		t.Fatalf("Failed to insert community gear: %v", err)
	}

	// Create community event.
	event := &models.CommunityEvent{
		Id:          uuid.New().String(),
		CommunityId: community.Id,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:     user.Id,
		GearId:      gear.Id,
	}
	if _, err := db.Insert(ctx, event); err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Create a transfer.
	transfer := &models.Transfer{
		Id:          uuid.New().String(),
		CommunityId: community.Id,
		OwnerId:     user.Id,
		GearId:      gear.Id,
	}
	if _, err := db.Insert(ctx, transfer); err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}

	// Run cleanup.
	resp, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: simID,
	}))
	if err != nil {
		t.Fatalf("CleanupSimulation failed: %v", err)
	}

	if resp.Msg.UsersDeleted != 1 {
		t.Errorf("Expected 1 user deleted, got %d", resp.Msg.UsersDeleted)
	}
	if resp.Msg.CommunitiesDeleted != 1 {
		t.Errorf("Expected 1 community deleted, got %d", resp.Msg.CommunitiesDeleted)
	}
	// user + community + membership + gear + community_gear + event + transfer = 7
	if resp.Msg.RecordsDeleted < 7 {
		t.Errorf("Expected at least 7 records deleted, got %d", resp.Msg.RecordsDeleted)
	}

	// Verify entities are gone.
	err = db.GetByID(ctx, user.Id, &models.User{})
	if err == nil {
		t.Error("Expected user to be deleted")
	}
	err = db.GetByID(ctx, community.Id, &models.Community{})
	if err == nil {
		t.Error("Expected community to be deleted")
	}
	err = db.GetByID(ctx, gear.Id, &models.Gear{})
	if err == nil {
		t.Error("Expected gear to be deleted")
	}
	msgs, err := db.QueryByField(ctx, "community_id", community.Id, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Expected 0 events, got %d", len(msgs))
	}
}

// TestCleanupSimulation_PreservesNonSimulationData verifies that cleanup does not
// affect data without a matching simulation_id.
func TestCleanupSimulation_PreservesNonSimulationData(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)
	simID := "sim-2026-02-24-preserve-test"

	// Create a real (non-simulation) user.
	realUser := &models.User{
		Id:    uuid.New().String(),
		Name:  "Real User",
		Email: "real@test.com",
	}
	if _, err := db.Insert(ctx, realUser); err != nil {
		t.Fatalf("Failed to insert real user: %v", err)
	}

	// Create a real community.
	realCommunity := &models.Community{
		Id:          uuid.New().String(),
		Name:        "Real Community",
		CreatorId:   realUser.Id,
		OwnerUserId: realUser.Id,
	}
	if _, err := db.Insert(ctx, realCommunity); err != nil {
		t.Fatalf("Failed to insert real community: %v", err)
	}

	// Create real gear.
	realGear := &models.Gear{
		Id:      uuid.New().String(),
		Name:    "Real Drill",
		OwnerId: realUser.Id,
	}
	if _, err := db.Insert(ctx, realGear); err != nil {
		t.Fatalf("Failed to insert real gear: %v", err)
	}

	// Create simulation data to clean up.
	simUser := &models.User{
		Id:           uuid.New().String(),
		Name:         "Sim User",
		Email:        "sim@test.com",
		SimulationId: proto.String(simID),
	}
	if _, err := db.Insert(ctx, simUser); err != nil {
		t.Fatalf("Failed to insert sim user: %v", err)
	}

	// Run cleanup.
	resp, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: simID,
	}))
	if err != nil {
		t.Fatalf("CleanupSimulation failed: %v", err)
	}

	if resp.Msg.UsersDeleted != 1 {
		t.Errorf("Expected 1 user deleted, got %d", resp.Msg.UsersDeleted)
	}

	// Verify real data is untouched.
	err = db.GetByID(ctx, realUser.Id, &models.User{})
	if err != nil {
		t.Errorf("Real user should still exist: %v", err)
	}
	err = db.GetByID(ctx, realCommunity.Id, &models.Community{})
	if err != nil {
		t.Errorf("Real community should still exist: %v", err)
	}
	err = db.GetByID(ctx, realGear.Id, &models.Gear{})
	if err != nil {
		t.Errorf("Real gear should still exist: %v", err)
	}
}

// TestCleanupSimulation_FailsWithoutDevMode verifies that cleanup is rejected
// when dev mode is not enabled.
func TestCleanupSimulation_FailsWithoutDevMode(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, false)

	_, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: "sim-2026-02-24-test",
	}))
	if err == nil {
		t.Fatal("Expected error when dev mode is off")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", connect.CodeOf(err))
	}
}

// TestCleanupSimulation_RequiresSimulationID verifies that an empty simulation_id
// is rejected.
func TestCleanupSimulation_RequiresSimulationID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)

	_, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: "",
	}))
	if err == nil {
		t.Fatal("Expected error for empty simulation_id")
	}
}

// TestCleanupSimulation_NoMatchingData verifies that cleanup succeeds when
// no data matches the simulation_id.
func TestCleanupSimulation_NoMatchingData(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)

	resp, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: "sim-nonexistent",
	}))
	if err != nil {
		t.Fatalf("CleanupSimulation failed: %v", err)
	}

	if resp.Msg.UsersDeleted != 0 {
		t.Errorf("Expected 0 users deleted, got %d", resp.Msg.UsersDeleted)
	}
	if resp.Msg.CommunitiesDeleted != 0 {
		t.Errorf("Expected 0 communities deleted, got %d", resp.Msg.CommunitiesDeleted)
	}
	if resp.Msg.RecordsDeleted != 0 {
		t.Errorf("Expected 0 records deleted, got %d", resp.Msg.RecordsDeleted)
	}
}

// TestListSimulations_ReturnsAllSimulationIDs verifies that ListSimulations
// returns all distinct simulation IDs with correct user and community counts.
func TestListSimulations_ReturnsAllSimulationIDs(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)

	// Create data for two different simulation IDs.
	simID1 := "sim-scenario-a"
	simID2 := "sim-scenario-b"

	// simID1: 2 users, 1 community.
	for i := range 2 {
		user := &models.User{
			Id:           uuid.New().String(),
			Name:         fmt.Sprintf("User A%d", i),
			SimulationId: proto.String(simID1),
		}
		if _, err := db.Insert(ctx, user); err != nil {
			t.Fatalf("Failed to insert user: %v", err)
		}
	}
	community1 := &models.Community{
		Id:           uuid.New().String(),
		Name:         "Community A",
		CreatorId:    "test-user-a",
		OwnerUserId:  "test-user-a",
		SimulationId: proto.String(simID1),
	}
	if _, err := db.Insert(ctx, community1); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// simID2: 1 user, 2 communities.
	user2 := &models.User{
		Id:           uuid.New().String(),
		Name:         "User B",
		SimulationId: proto.String(simID2),
	}
	if _, err := db.Insert(ctx, user2); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}
	for i := range 2 {
		community := &models.Community{
			Id:           uuid.New().String(),
			Name:         fmt.Sprintf("Community B%d", i),
			CreatorId:    user2.Id,
			OwnerUserId:  user2.Id,
			SimulationId: proto.String(simID2),
		}
		if _, err := db.Insert(ctx, community); err != nil {
			t.Fatalf("Failed to insert community: %v", err)
		}
	}

	resp, err := adminService.ListSimulations(ctx, connect.NewRequest(&api.ListSimulationsRequest{}))
	if err != nil {
		t.Fatalf("ListSimulations failed: %v", err)
	}

	if len(resp.Msg.Simulations) != 2 {
		t.Fatalf("Expected 2 simulations, got %d", len(resp.Msg.Simulations))
	}

	// Build a map for easier assertion.
	byID := make(map[string]*api.SimulationInfo)
	for _, s := range resp.Msg.Simulations {
		byID[s.SimulationId] = s
	}

	if info, ok := byID[simID1]; !ok {
		t.Errorf("Missing simulation %s", simID1)
	} else {
		if info.UserCount != 2 {
			t.Errorf("%s: expected 2 users, got %d", simID1, info.UserCount)
		}
		if info.CommunityCount != 1 {
			t.Errorf("%s: expected 1 community, got %d", simID1, info.CommunityCount)
		}
	}

	if info, ok := byID[simID2]; !ok {
		t.Errorf("Missing simulation %s", simID2)
	} else {
		if info.UserCount != 1 {
			t.Errorf("%s: expected 1 user, got %d", simID2, info.UserCount)
		}
		if info.CommunityCount != 2 {
			t.Errorf("%s: expected 2 communities, got %d", simID2, info.CommunityCount)
		}
	}
}

// TestListSimulations_Empty verifies that an empty database returns no simulations.
func TestListSimulations_Empty(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)

	resp, err := adminService.ListSimulations(ctx, connect.NewRequest(&api.ListSimulationsRequest{}))
	if err != nil {
		t.Fatalf("ListSimulations failed: %v", err)
	}

	if len(resp.Msg.Simulations) != 0 {
		t.Errorf("Expected 0 simulations, got %d", len(resp.Msg.Simulations))
	}
}

// TestListSimulations_FailsWithoutDevMode verifies that ListSimulations is rejected
// when dev mode is not enabled.
func TestListSimulations_FailsWithoutDevMode(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, false)

	_, err := adminService.ListSimulations(ctx, connect.NewRequest(&api.ListSimulationsRequest{}))
	if err == nil {
		t.Fatal("Expected error when dev mode is off")
	}
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got: %v", connect.CodeOf(err))
	}
}

// TestCleanupSimulation_Idempotent verifies that running cleanup twice
// succeeds and the second run is a no-op.
func TestCleanupSimulation_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := setupTestDatabase(t)
	defer db.Close()

	adminService := NewService(db, true)
	simID := "sim-2026-02-24-idempotent"

	// Create simulation data.
	user := &models.User{
		Id:           uuid.New().String(),
		Name:         "Sim User",
		SimulationId: proto.String(simID),
	}
	if _, err := db.Insert(ctx, user); err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	community := &models.Community{
		Id:           uuid.New().String(),
		Name:         "Sim Community",
		CreatorId:    user.Id,
		OwnerUserId:  user.Id,
		SimulationId: proto.String(simID),
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// First cleanup.
	resp, err := adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: simID,
	}))
	if err != nil {
		t.Fatalf("First cleanup failed: %v", err)
	}
	if resp.Msg.UsersDeleted != 1 || resp.Msg.CommunitiesDeleted != 1 {
		t.Errorf("First cleanup: expected 1 user + 1 community deleted, got %d + %d",
			resp.Msg.UsersDeleted, resp.Msg.CommunitiesDeleted)
	}

	// Second cleanup — should be a no-op.
	resp, err = adminService.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
		SimulationId: simID,
	}))
	if err != nil {
		t.Fatalf("Second cleanup failed: %v", err)
	}
	if resp.Msg.UsersDeleted != 0 || resp.Msg.CommunitiesDeleted != 0 {
		t.Errorf("Second cleanup: expected 0 deleted, got %d users + %d communities",
			resp.Msg.UsersDeleted, resp.Msg.CommunitiesDeleted)
	}
}
