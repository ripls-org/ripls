package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGetFeed_TransferInterestExpressedAppearsAsGearCard verifies that a
// TRANSFER_INTEREST_EXPRESSED event surfaces as a gear card.
func TestGetFeed_TransferInterestExpressedAppearsAsGearCard(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "lifecycle@test.com", "Lifecycle User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Lifecycle Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Lifecycle Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	eventID := createTransferEvent(t, sqlStorage, communityID, userID, gearID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED)

	ctx := createAuthenticatedContext(userID, "lifecycle@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item for TRANSFER_INTEREST_EXPRESSED, got %d", len(resp.Msg.Items))
	}
	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected event ID %s, got %s", eventID, item.Id)
	}
	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
		t.Errorf("Expected GEAR_SHARED card, got %v", item.ItemType)
	}
	if item.GetGearShared() == nil || item.GetGearShared().GearId != gearID {
		t.Errorf("Expected gear payload with gear_id %s", gearID)
	}
}

// TestGetFeed_RequestOfferMadeAppearsAsRequestCard verifies that a
// REQUEST_OFFER_MADE event surfaces as a request card.
func TestGetFeed_RequestOfferMadeAppearsAsRequestCard(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "lifecycle2@test.com", "Lifecycle2 User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Lifecycle2 Community")
	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Lifecycle2 Request", models.RequestState_REQUEST_STATE_ACTIVE)

	createRequestLifecycleEvent(t, sqlStorage, communityID, userID, requestID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE)

	ctx := createAuthenticatedContext(userID, "lifecycle2@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item for REQUEST_OFFER_MADE, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].ItemType != api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED {
		t.Errorf("Expected REQUEST_CREATED card, got %v", resp.Msg.Items[0].ItemType)
	}
	if resp.Msg.Items[0].GetRequestCreated() == nil || resp.Msg.Items[0].GetRequestCreated().RequestId != requestID {
		t.Errorf("Expected request payload with request_id %s", requestID)
	}
}

// TestGetFeed_ExperienceRSVPAppearsAsExperienceCard verifies that an
// EXPERIENCE_RSVP_YES event surfaces as an experience card.
func TestGetFeed_ExperienceRSVPAppearsAsExperienceCard(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "lifecycle3@test.com", "Lifecycle3 User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Lifecycle3 Community")
	experienceID := setupTestExperience(t, sqlStorage, userID, "Lifecycle3 Experience")
	setupCommunityExperience(t, sqlStorage, communityID, experienceID, false)

	createExperienceRSVPEvent(t, sqlStorage, communityID, userID, experienceID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES)

	ctx := createAuthenticatedContext(userID, "lifecycle3@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 item for EXPERIENCE_RSVP_YES, got %d", len(resp.Msg.Items))
	}
	if resp.Msg.Items[0].ItemType != api.FeedItemType_FEED_ITEM_TYPE_EXPERIENCE_CREATED {
		t.Errorf("Expected EXPERIENCE_CREATED card, got %v", resp.Msg.Items[0].ItemType)
	}
	payload := resp.Msg.Items[0].GetExperienceCreated()
	if payload == nil || payload.ExperienceId != experienceID {
		t.Errorf("Expected experience payload with experience_id %s", experienceID)
	}
}

// TestGetFeed_TerminalTransferEventsSkipped verifies that completed/cancelled
// transfer events do not generate feed items (they appear as Story cards instead).
func TestGetFeed_TerminalTransferEventsSkipped(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "terminal@test.com", "Terminal User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Terminal Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Terminal Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	createTransferEvent(t, sqlStorage, communityID, userID, gearID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED)
	createTransferEvent(t, sqlStorage, communityID, userID, gearID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED)

	ctx := createAuthenticatedContext(userID, "terminal@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 items for terminal events, got %d", len(resp.Msg.Items))
	}
}

// --- Phase 3: deduplication + activity fields ---.

// TestGetFeed_DeduplicatesGearEvents verifies that multiple events for the same
// gear item collapse into a single feed card showing the most recent event.
func TestGetFeed_DeduplicatesGearEvents(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "dedup@test.com", "Dedup User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Dedup Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Dedup Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create two events for the same gear: GEAR_SHARED first, then TRANSFER_INTEREST_EXPRESSED later.
	// Use explicit timestamps so the second event is unambiguously more recent.
	baseTime := time.Now().Unix()
	olderEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime - 60, // 1 minute ago
	}
	if _, err := sqlStorage.Insert(context.Background(), olderEvent); err != nil {
		t.Fatalf("failed to create gear shared event: %v", err)
	}
	laterEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: baseTime, // now
	}
	laterEventID, err := sqlStorage.Insert(context.Background(), laterEvent)
	if err != nil {
		t.Fatalf("failed to create transfer event: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "dedup@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Should collapse to exactly 1 item.
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 deduplicated item, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	// The latest event should be the TRANSFER_INTEREST_EXPRESSED one.
	if item.Id != laterEventID {
		t.Errorf("Expected most-recent event ID %s, got %s", laterEventID, item.Id)
	}
	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
		t.Errorf("Expected GEAR_SHARED card type, got %v", item.ItemType)
	}
	// Item has never been viewed, so it should be unread.
	if !item.IsUnread {
		t.Error("Expected item to be unread (never seen)")
	}
}

// TestGetFeed_DeduplicatesRequestEvents verifies that multiple request lifecycle
// events collapse into a single feed card.
func TestGetFeed_DeduplicatesRequestEvents(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "dedup2@test.com", "Dedup2 User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Dedup2 Community")
	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Need a tent", models.RequestState_REQUEST_STATE_ACTIVE)

	// Create REQUEST_CREATED then REQUEST_OFFER_MADE.
	createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)
	createRequestLifecycleEvent(t, sqlStorage, communityID, userID, requestID,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE)

	ctx := createAuthenticatedContext(userID, "dedup2@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 deduplicated item, got %d", len(resp.Msg.Items))
	}
	if !resp.Msg.Items[0].IsUnread {
		t.Error("Expected deduplicated item to be unread (never seen)")
	}
}

// TestItemKeyForEvent verifies that itemKeyForEvent groups events correctly.
func TestItemKeyForEvent(t *testing.T) {
	gearID := "gear-abc"
	requestID := "req-abc"
	experienceID := "exp-abc"
	communityID := "comm-abc"

	tests := []struct {
		name    string
		event   *models.CommunityEvent
		wantKey string
	}{
		{
			name:    "GEAR_SHARED",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED, GearId: gearID},
			wantKey: "gear:" + gearID,
		},
		{
			name:    "TRANSFER_INTEREST_EXPRESSED",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED, GearId: gearID},
			wantKey: "gear:" + gearID,
		},
		{
			name:    "REQUEST_CREATED",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED, Topic: &models.CommunityEvent_RequestId{RequestId: requestID}},
			wantKey: "request:" + requestID,
		},
		{
			name:    "REQUEST_OFFER_MADE",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE, Topic: &models.CommunityEvent_RequestId{RequestId: requestID}},
			wantKey: "request:" + requestID,
		},
		{
			name:    "EXPERIENCE_CREATED",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED, Topic: &models.CommunityEvent_ExperienceId{ExperienceId: experienceID}},
			wantKey: "experience:" + experienceID,
		},
		{
			name:    "EXPERIENCE_RSVP_YES",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES, Topic: &models.CommunityEvent_ExperienceId{ExperienceId: experienceID}},
			wantKey: "experience:" + experienceID,
		},
		{
			name:    "COMMUNITY_CREATED",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED, CommunityId: communityID},
			wantKey: "community:" + communityID,
		},
		{
			name:    "unhandled type returns empty",
			event:   &models.CommunityEvent{EventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT},
			wantKey: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := itemKeyForEvent(tc.event)
			if got != tc.wantKey {
				t.Errorf("itemKeyForEvent() = %q, want %q", got, tc.wantKey)
			}
		})
	}
}
