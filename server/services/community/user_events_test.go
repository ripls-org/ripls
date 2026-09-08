package community

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestListUserEvents_SpansEveryCommunity is the point of the RPC: one request
// answers for the caller's whole portfolio. The per-community backstop it
// replaced needed one request per community per interval.
func TestListUserEvents_SpansEveryCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	var communityIDs []string
	for _, name := range []string{"First", "Second", "Third"} {
		resp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
			Name:        name,
			Description: "test",
		}))
		if err != nil {
			t.Fatalf("failed to create community %s: %v", name, err)
		}
		communityIDs = append(communityIDs, resp.Msg.Id)
	}

	// One gear share per community, so each has an event of its own.
	for _, communityID := range communityIDs {
		gearID, err := testStorage.Insert(ctxAlice, &models.Gear{
			Name:    "Gear for " + communityID,
			OwnerId: aliceID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		})
		if err != nil {
			t.Fatalf("failed to create gear: %v", err)
		}
		shareGearForTestWithAvailability(
			t, service, ctxAlice, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)
	}

	resp, err := service.ListUserEvents(ctxAlice, connect.NewRequest(&api.ListUserEventsRequest{}))
	if err != nil {
		t.Fatalf("ListUserEvents failed: %v", err)
	}

	seen := map[string]bool{}
	for _, event := range resp.Msg.Events {
		seen[event.CommunityId] = true
	}
	for _, communityID := range communityIDs {
		if !seen[communityID] {
			t.Errorf("expected an event from community %s in the portfolio-wide listing", communityID)
		}
	}
}

// TestListUserEvents_ExcludesOtherPeoplesCommunities pins the membership
// filter. The request names no community, so this is the only thing keeping one
// user's poll from returning another's activity.
func TestListUserEvents_ExcludesOtherPeoplesCommunities(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	strangerID := setupTestUser(t, testStorage, "stranger@example.com", "Stranger")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxStranger := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Alice's Community",
		Description: "stranger is not a member",
	}))
	if err != nil {
		t.Fatalf("failed to create community: %v", err)
	}

	gearID, err := testStorage.Insert(ctxAlice, &models.Gear{
		Name:    "Alice's Gear",
		OwnerId: aliceID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("failed to create gear: %v", err)
	}
	shareGearForTestWithAvailability(
		t, service, ctxAlice, gearID, createResp.Msg.Id, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	resp, err := service.ListUserEvents(ctxStranger, connect.NewRequest(&api.ListUserEventsRequest{}))
	if err != nil {
		t.Fatalf("ListUserEvents failed: %v", err)
	}

	for _, event := range resp.Msg.Events {
		if event.CommunityId == createResp.Msg.Id {
			t.Fatalf("a non-member's poll returned an event from %s", createResp.Msg.Id)
		}
	}
}

func TestListUserEvents_SinceFiltersOlderEvents(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "test",
	}))
	if err != nil {
		t.Fatalf("failed to create community: %v", err)
	}

	gearID, err := testStorage.Insert(ctxAlice, &models.Gear{
		Name:    "Test Gear",
		OwnerId: aliceID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("failed to create gear: %v", err)
	}
	shareGearForTestWithAvailability(
		t, service, ctxAlice, gearID, createResp.Msg.Id, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	all, err := service.ListUserEvents(ctxAlice, connect.NewRequest(&api.ListUserEventsRequest{}))
	if err != nil {
		t.Fatalf("ListUserEvents failed: %v", err)
	}
	if len(all.Msg.Events) == 0 {
		t.Fatal("expected at least one event before filtering")
	}

	// Ask for events strictly newer than the newest one — should be empty.
	var newest int64
	for _, event := range all.Msg.Events {
		newest = max(newest, event.OccurredAtUnixSec)
	}
	filtered, err := service.ListUserEvents(ctxAlice, connect.NewRequest(&api.ListUserEventsRequest{
		SinceUnixSec: &newest,
	}))
	if err != nil {
		t.Fatalf("ListUserEvents with since failed: %v", err)
	}
	if len(filtered.Msg.Events) != 0 {
		t.Errorf("expected no events newer than the newest, got %d", len(filtered.Msg.Events))
	}
}

func TestListUserEvents_OrderedOldestFirst(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "test",
	}))
	if err != nil {
		t.Fatalf("failed to create community: %v", err)
	}

	for i := range 3 {
		gearID, err := testStorage.Insert(ctxAlice, &models.Gear{
			Name:    "Gear",
			OwnerId: aliceID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		})
		if err != nil {
			t.Fatalf("failed to create gear %d: %v", i, err)
		}
		shareGearForTestWithAvailability(
			t, service, ctxAlice, gearID, createResp.Msg.Id, models.Availability_AVAILABILITY_FOR_LOAN,
		)
	}

	resp, err := service.ListUserEvents(ctxAlice, connect.NewRequest(&api.ListUserEventsRequest{}))
	if err != nil {
		t.Fatalf("ListUserEvents failed: %v", err)
	}

	// Ascending order is what lets a client advance its high-water mark as it
	// walks the slice, and what makes a truncated page resumable.
	for i := 1; i < len(resp.Msg.Events); i++ {
		if resp.Msg.Events[i].OccurredAtUnixSec < resp.Msg.Events[i-1].OccurredAtUnixSec {
			t.Fatalf("events out of order at index %d: %d < %d",
				i, resp.Msg.Events[i].OccurredAtUnixSec, resp.Msg.Events[i-1].OccurredAtUnixSec)
		}
	}
}
