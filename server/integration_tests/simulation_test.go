package integration_tests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/simulation"
	"go.ripls.org/ripls/server/storage"
)

// TestSimulation_RunnerSetup verifies that the simulation runner can register
// users, create a community, invite members, create gear, and execute a
// timeline of multi-user activity via standard RPCs.
func TestSimulation_RunnerSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	_, serverURL := startTestServer(t, dbURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	scenario := simulation.Scenario{
		Name:        "test-runner",
		Description: "Minimal scenario for runner integration test",
		StartTime:   time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
		Communities: []simulation.CommunityDef{
			{
				Name:        "Test Community",
				Description: "A small test community",
				Members: []simulation.MemberDef{
					{Name: "Alice Alfred", Email: "alice@sim.test", Persona: simulation.PersonaAlfred},
					{Name: "Bob Derek", Email: "bob@sim.test", Persona: simulation.PersonaDerek},
					{Name: "Carol Gary", Email: "carol@sim.test", Persona: simulation.PersonaGary},
				},
			},
		},
	}

	// Resolve testdata assets directory (contains one image per category).
	assetsDir, err := filepath.Abs("../simulation/testdata")
	if err != nil {
		t.Fatalf("resolve testdata path: %v", err)
	}

	cfg := simulation.RunConfig{
		ServerURL: serverURL,
		Scenario:  scenario,
		Seed:      42,
		AssetsDir: assetsDir,
	}

	result, err := simulation.RunSimulation(ctx, cfg)
	if err != nil {
		t.Fatalf("RunSimulation failed: %v", err)
	}

	// Verify user and community creation.
	if result.UsersCreated != 3 {
		t.Errorf("UsersCreated = %d, want 3", result.UsersCreated)
	}
	if result.CommunitiesCreated != 1 {
		t.Errorf("CommunitiesCreated = %d, want 1", result.CommunitiesCreated)
	}
	if result.SimulationID != scenario.SimulationID() {
		t.Errorf("SimulationID = %q, want %q", result.SimulationID, scenario.SimulationID())
	}
	if result.TimelineSteps == 0 {
		t.Error("TimelineSteps = 0, want > 0")
	}

	// Verify gear was created.
	if result.State.GearCreated == 0 {
		t.Error("GearCreated = 0, want > 0")
	}

	// Verify media was uploaded (testdata has 1 image per category = 4 total).
	if len(result.State.MediaIDs) == 0 {
		t.Error("MediaIDs is empty, expected uploaded media")
	}
	t.Logf("  Media uploaded: %d", len(result.State.MediaIDs))

	// Verify community members via RPC.
	aliceToken := result.State.UserTokens["alice@sim.test"]
	if aliceToken == "" {
		t.Fatal("Alice's auth token not found in state")
	}

	communityID := result.State.CommunityIDs["Test Community"]
	if communityID == "" {
		t.Fatal("Community ID not found in state")
	}

	client := simulation.NewClient(serverURL)
	client.AsUser(aliceToken)

	membersResp, err := client.Community().ListCommunityUsers(ctx, connect.NewRequest(&api.ListCommunityUsersRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListCommunityUsers failed: %v", err)
	}
	if len(membersResp.Msg.Members) != 3 {
		t.Errorf("Community members = %d, want 3", len(membersResp.Msg.Members))
	}

	// Verify at least one gear item has a media ID attached.
	gearResp, err := client.Community().ListCommunityGear(ctx, connect.NewRequest(&api.ListCommunityGearRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListCommunityGear failed: %v", err)
	}

	gearWithMedia := 0
	for _, item := range gearResp.Msg.GearItems {
		if len(item.MediaIds) > 0 {
			gearWithMedia++
		}
	}
	if gearWithMedia == 0 {
		t.Error("no gear items have media IDs, expected at least one")
	}
	t.Logf("  Gear with media: %d/%d", gearWithMedia, len(gearResp.Msg.GearItems))

	// Log summary.
	t.Logf("Simulation %q complete in %v:", result.SimulationID, result.Duration)
	t.Logf("  Users: %d, Communities: %d, Timeline steps: %d",
		result.UsersCreated, result.CommunitiesCreated, result.TimelineSteps)
	t.Logf("  Gear: %d", result.State.GearCreated)
	t.Logf("  Transfers started/completed: %d/%d",
		result.State.TransfersStarted, result.State.TransfersCompleted)
	t.Logf("  Requests submitted/fulfilled: %d/%d",
		result.State.RequestsSubmitted, result.State.RequestsFulfilled)
	t.Logf("  Experiences created/completed: %d/%d",
		result.State.ExperiencesCreated, result.State.ExperiencesCompleted)
	t.Logf("  RSVPs (yes/no): %d/%d", result.State.RSVPYesCount, result.State.RSVPNoCount)
	t.Logf("  Cancelled: transfers=%d, requests=%d, experiences=%d",
		result.State.TransfersCancelled, result.State.RequestsCancelled, result.State.ExperiencesCancelled)
	t.Logf("  Withdrawn: interest=%d, offers=%d",
		result.State.InterestWithdrawn, result.State.OffersWithdrawn)
	t.Logf("  Steps skipped: %d, failed: %d", result.State.StepsSkipped, result.State.StepsFailed)
}

// TestSimulation_CleanupOnRerun verifies that running the same simulation
// twice cleans up the previous run's data before creating new entities.
func TestSimulation_CleanupOnRerun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dbURL, cleanup := storage.SetupTestDatabase(t)
	defer cleanup()

	_, serverURL := startTestServer(t, dbURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	scenario := simulation.Scenario{
		Name:        "test-cleanup",
		Description: "Scenario for cleanup testing",
		StartTime:   time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC),
		Communities: []simulation.CommunityDef{
			{
				Name:        "Cleanup Test Community",
				Description: "Community for cleanup test",
				Members: []simulation.MemberDef{
					{Name: "Alpha", Email: "alpha@sim.test", Persona: simulation.PersonaAlfred},
					{Name: "Beta", Email: "beta@sim.test", Persona: simulation.PersonaDerek},
				},
			},
		},
	}

	cfg := simulation.RunConfig{
		ServerURL: serverURL,
		Scenario:  scenario,
		Seed:      42,
	}

	// Run the simulation the first time.
	result1, err := simulation.RunSimulation(ctx, cfg)
	if err != nil {
		t.Fatalf("First RunSimulation failed: %v", err)
	}
	if result1.UsersCreated != 2 {
		t.Fatalf("First run: UsersCreated = %d, want 2", result1.UsersCreated)
	}
	t.Logf("First run: %d users, %d communities, %d gear",
		result1.UsersCreated, result1.CommunitiesCreated, result1.State.GearCreated)

	// Run the same simulation again — cleanup should remove previous data.
	result2, err := simulation.RunSimulation(ctx, cfg)
	if err != nil {
		t.Fatalf("Second RunSimulation failed: %v", err)
	}
	if result2.UsersCreated != 2 {
		t.Fatalf("Second run: UsersCreated = %d, want 2", result2.UsersCreated)
	}
	t.Logf("Second run: %d users, %d communities, %d gear",
		result2.UsersCreated, result2.CommunitiesCreated, result2.State.GearCreated)

	// Verify the community only has 2 members (not 4 from two runs).
	communityID := result2.State.CommunityIDs["Cleanup Test Community"]
	if communityID == "" {
		t.Fatal("Community ID not found after second run")
	}

	client := simulation.NewClient(serverURL)
	client.AsUser(result2.State.UserTokens["alpha@sim.test"])

	membersResp, err := client.Community().ListCommunityUsers(ctx, connect.NewRequest(&api.ListCommunityUsersRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListCommunityUsers failed: %v", err)
	}
	if len(membersResp.Msg.Members) != 2 {
		t.Errorf("Community has %d members after rerun, want 2 (cleanup should have removed previous run's users)",
			len(membersResp.Msg.Members))
	}
}
