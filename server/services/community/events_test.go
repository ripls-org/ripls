package community

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// =============================================================================
// COMMUNITY EVENTS TESTS
// =============================================================================.

func TestService_ListCommunityEvents(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice Smith")
	bobID := setupTestUser(t, testStorage, "bob@example.com", "Bob Jones")
	nonMemberID := setupTestUser(t, testStorage, "nonmember@example.com", "Non Member")

	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)
	ctxNonMember := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

	// Alice creates a community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "A test community",
	})
	createResp, err := service.CreateCommunity(ctxAlice, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Add Bob to community using invitation link
	addUserToCommunity(t, service, communityID, aliceID, bobID, "alice@example.com", "bob@example.com")

	// Create and share gear
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: aliceID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := testStorage.Insert(ctxAlice, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	shareGearForTestWithAvailability(
		t, service, ctxAlice, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	t.Run("member can list all community events", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		// We should have at least 2 events: INVITATION_LINK_USED, GEAR_SHARED
		if len(listResp.Msg.Events) < 2 {
			t.Fatalf("Expected at least 2 events, got %d", len(listResp.Msg.Events))
		}

		// Verify all events have required fields populated
		for _, event := range listResp.Msg.Events {
			if event.Id == "" {
				t.Error("Expected event ID to be set")
			}
			if event.CommunityId != communityID {
				t.Errorf("Expected community ID %s, got %s", communityID, event.CommunityId)
			}
			if event.Actor == nil {
				t.Error("Expected actor to be set")
			} else {
				if event.Actor.Id == "" {
					t.Error("Expected actor ID to be set")
				}
				if event.Actor.Name == "" {
					t.Error("Expected actor name to be set")
				}
			}
			if event.OccurredAtUnixSec == 0 {
				t.Error("Expected occurred_at_unix_sec to be set")
			}
		}

		// Find and verify specific events
		eventTypes := make(map[api.CommunityEventType]bool)
		for _, event := range listResp.Msg.Events {
			eventTypes[event.EventType] = true
		}

		if !eventTypes[api.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED] {
			t.Error("Expected INVITATION_LINK_USED event")
		}
		if !eventTypes[api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED] {
			t.Error("Expected GEAR_SHARED event")
		}
	})

	t.Run("can filter by single event type", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
			EventTypes: []api.CommunityEventType{
				api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			},
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		// Should only have GEAR_SHARED events
		for _, event := range listResp.Msg.Events {
			if event.EventType != api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED {
				t.Errorf("Expected only GEAR_SHARED events, got %v", event.EventType)
			}
		}

		if len(listResp.Msg.Events) != 1 {
			t.Errorf("Expected 1 GEAR_SHARED event, got %d", len(listResp.Msg.Events))
		}
	})

	t.Run("can filter by multiple event types", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
			EventTypes: []api.CommunityEventType{
				api.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
				api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			},
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		// Should only have invitation link and gear shared events
		for _, event := range listResp.Msg.Events {
			if event.EventType != api.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED &&
				event.EventType != api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED {
				t.Errorf("Expected only INVITATION_LINK_USED or GEAR_SHARED events, got %v", event.EventType)
			}
		}

		if len(listResp.Msg.Events) != 2 {
			t.Errorf("Expected 2 invitation events, got %d", len(listResp.Msg.Events))
		}
	})

	t.Run("non-member cannot list community events", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
		})

		_, err := service.ListCommunityEvents(ctxNonMember, listReq)
		if err == nil {
			t.Fatal("Expected error when non-member tries to list events")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})

	t.Run("empty list when no events match filter", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
			EventTypes: []api.CommunityEventType{
				api.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT,
			},
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		if len(listResp.Msg.Events) != 0 {
			t.Errorf("Expected 0 events for MEMBER_LEFT filter, got %d", len(listResp.Msg.Events))
		}
	})

	t.Run("since_unix_sec filters to newer events only", func(t *testing.T) {
		// All events created above have recent timestamps.
		// Insert an old event directly into storage.
		oldEvent := &models.CommunityEvent{
			CommunityId:       communityID,
			EventType:         models.CommunityEventType(api.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT),
			ActorId:           bobID,
			OccurredAtUnixSec: time.Now().Unix() - 3600, // 1 hour ago
		}
		oldID, err := testStorage.Insert(ctxAlice, oldEvent)
		if err != nil {
			t.Fatalf("Failed to insert old event: %v", err)
		}

		// Request events since 30 minutes ago — should exclude the old event.
		thirtyMinAgo := time.Now().Unix() - 1800
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId:  communityID,
			SinceUnixSec: proto.Int64(thirtyMinAgo),
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		for _, event := range listResp.Msg.Events {
			if event.Id == oldID {
				t.Error("Expected old event to be filtered out by since_unix_sec")
			}
		}
	})

	t.Run("nil since_unix_sec returns all events", func(t *testing.T) {
		// Without since_unix_sec, all events (including the old one above) should return.
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId: communityID,
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		// Should include the old MEMBER_LEFT event we inserted above.
		found := false
		for _, event := range listResp.Msg.Events {
			if event.EventType == api.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT {
				found = true
			}
		}
		if !found {
			t.Error("Expected all events including old MEMBER_LEFT when since_unix_sec is nil")
		}
	})

	t.Run("no N+1 — query count is constant regardless of event count", func(t *testing.T) {
		// Insert additional events with distinct actor/object-user/gear IDs so that
		// a naive per-event implementation would issue many queries.
		carolID := setupTestUser(t, testStorage, "carol@example.com", "Carol White")
		daveID := setupTestUser(t, testStorage, "dave@example.com", "Dave Black")
		extraGear := &models.Gear{
			Name:    "Extra Gear",
			OwnerId: carolID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		extraGearID, err := testStorage.Insert(ctxAlice, extraGear)
		if err != nil {
			t.Fatalf("Failed to insert extra gear: %v", err)
		}
		for _, evt := range []*models.CommunityEvent{
			{CommunityId: communityID, EventType: models.CommunityEventType(api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED), ActorId: carolID, GearId: extraGearID, OccurredAtUnixSec: time.Now().Unix()},
			{CommunityId: communityID, EventType: models.CommunityEventType(api.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED), ActorId: daveID, ObjectUserId: carolID, OccurredAtUnixSec: time.Now().Unix()},
		} {
			if _, err := testStorage.Insert(ctxAlice, evt); err != nil {
				t.Fatalf("Failed to insert event: %v", err)
			}
		}

		statsCtx := storage.WithQueryStats(ctxAlice)
		var listErr error
		storage.AssertMaxQueries(t, statsCtx, 6, func() {
			_, listErr = service.ListCommunityEvents(statsCtx, connect.NewRequest(&api.ListCommunityEventsRequest{
				CommunityId: communityID,
			}))
		})
		if listErr != nil {
			t.Fatalf("ListCommunityEvents failed: %v", listErr)
		}
	})

	t.Run("since_unix_sec caps at 7 days ago", func(t *testing.T) {
		// Insert an event 10 days ago.
		veryOldEvent := &models.CommunityEvent{
			CommunityId:       communityID,
			EventType:         models.CommunityEventType(api.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED),
			ActorId:           aliceID,
			OccurredAtUnixSec: time.Now().Unix() - 10*24*3600, // 10 days ago
		}
		veryOldID, err := testStorage.Insert(ctxAlice, veryOldEvent)
		if err != nil {
			t.Fatalf("Failed to insert very old event: %v", err)
		}

		// Request since 30 days ago — server should cap to 7 days, excluding the 10-day-old event.
		thirtyDaysAgo := time.Now().Unix() - 30*24*3600
		listReq := connect.NewRequest(&api.ListCommunityEventsRequest{
			CommunityId:  communityID,
			SinceUnixSec: proto.Int64(thirtyDaysAgo),
		})

		listResp, err := service.ListCommunityEvents(ctxAlice, listReq)
		if err != nil {
			t.Fatalf("ListCommunityEvents failed: %v", err)
		}

		for _, event := range listResp.Msg.Events {
			if event.Id == veryOldID {
				t.Error("Expected 10-day-old event to be excluded by 7-day cap")
			}
		}
	})
}

// TestService_ListCommunityEvents_DeletedActor verifies that
// ListCommunityEvents tolerates events whose actor or object_user has been
// soft-deleted (account deletion). Regression test for #1670 — the old
// behavior was a 500 that broke the entire community feed for every member
// until the offending event aged past the 7-day cap.
func TestService_ListCommunityEvents_DeletedActor(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	ctx := context.Background()

	aliceID := setupTestUser(t, testStorage, "alice@example.com", "Alice")
	ctxAlice := createAuthenticatedContext(aliceID, "alice@example.com", models.Role_ROLE_USER)

	// Create the community as Alice.
	createResp, err := service.CreateCommunity(ctxAlice, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Deleted Actor Test",
		Description: "Regression coverage for #1670",
	}))
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Create Ghost, then immediately soft-delete the user record.
	ghostID := setupTestUser(t, testStorage, "ghost@example.com", "Ghost User")
	ghost := &models.User{}
	if err := testStorage.GetByID(ctx, ghostID, ghost); err != nil {
		t.Fatalf("Failed to load ghost: %v", err)
	}
	ghost.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  ghostID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := testStorage.Update(ctx, ghost); err != nil {
		t.Fatalf("Failed to soft-delete ghost: %v", err)
	}

	// Insert a CommunityEvent whose actor is the deleted user.
	deletedActorEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           ghostID,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	deletedActorEventID, err := testStorage.Insert(ctx, deletedActorEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Insert a sibling CommunityEvent whose object_user is the deleted user.
	deletedObjectEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		ActorId:           aliceID,
		ObjectUserId:      ghostID,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	deletedObjectEventID, err := testStorage.Insert(ctx, deletedObjectEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Call ListCommunityEvents as Alice — must succeed (not 500).
	listResp, err := service.ListCommunityEvents(ctxAlice, connect.NewRequest(&api.ListCommunityEventsRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("ListCommunityEvents must tolerate deleted actor; got error: %v", err)
	}

	var sawActorEvent, sawObjectEvent bool
	for _, ev := range listResp.Msg.Events {
		switch ev.Id {
		case deletedActorEventID:
			sawActorEvent = true
			if ev.Actor == nil {
				t.Error("Actor must be a placeholder, not nil")
				continue
			}
			if !ev.Actor.FormerMember {
				t.Errorf("Actor.FormerMember = false, want true for deleted user")
			}
			if ev.Actor.Id != ghostID {
				t.Errorf("Actor.Id = %q, want %q", ev.Actor.Id, ghostID)
			}
			if ev.Actor.Name != "" {
				t.Errorf("Actor.Name = %q, want empty (no PII for former member)", ev.Actor.Name)
			}
		case deletedObjectEventID:
			sawObjectEvent = true
			if ev.ObjectUser == nil {
				t.Error("ObjectUser must be a placeholder, not nil")
				continue
			}
			if !ev.ObjectUser.FormerMember {
				t.Errorf("ObjectUser.FormerMember = false, want true for deleted user")
			}
			if ev.ObjectUser.Id != ghostID {
				t.Errorf("ObjectUser.Id = %q, want %q", ev.ObjectUser.Id, ghostID)
			}
		}
	}
	if !sawActorEvent {
		t.Error("Expected the deleted-actor event in response, but did not find it")
	}
	if !sawObjectEvent {
		t.Error("Expected the deleted-object-user event in response, but did not find it")
	}
}

// TestModelEventTypeToAPI_ConvertsByName guards against the numeric-cast bug
// where diverging enum numbering mislabeled event types on the wire (storage
// REQUEST_FULFILLED=13 surfaced as API REQUEST_OFFER_SELECTED=13). Every
// storage value whose name exists in the API enum must map to the same-named
// API value; storage-only values map to UNSPECIFIED.
func TestModelEventTypeToAPI_ConvertsByName(t *testing.T) {
	for num, name := range models.CommunityEventType_name {
		modelType := models.CommunityEventType(num)
		got := ModelEventTypeToAPI(modelType)
		if apiNum, ok := api.CommunityEventType_value[name]; ok {
			if int32(got) != apiNum {
				t.Errorf("%s: mapped to %s, expected same-named API value", name, got)
			}
		} else if got != api.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED {
			t.Errorf("%s: storage-only type should map to UNSPECIFIED, got %s", name, got)
		}
	}

	// The concrete mislabel the numeric cast produced.
	if got := ModelEventTypeToAPI(models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED); got != api.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED {
		t.Errorf("REQUEST_FULFILLED mapped to %s", got)
	}
}
