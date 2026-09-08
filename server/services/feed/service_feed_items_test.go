package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetFeed_IncludesRequestCreated(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Looking for a drill", models.RequestState_REQUEST_STATE_ACTIVE)
	eventID := createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)

	// Create authenticated context
	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Call GetFeed
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	// Assertions
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected item ID %s, got %s", eventID, item.Id)
	}

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED {
		t.Errorf("Expected REQUEST_CREATED type, got %v", item.ItemType)
	}

	if !item.IsUnread {
		t.Error("Expected item to be unread (never seen)")
	}

	payload := item.GetRequestCreated()
	if payload == nil {
		t.Fatal("Expected request created payload")
	}

	if payload.RequestId != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, payload.RequestId)
	}

	if payload.Description != "Looking for a drill" {
		t.Errorf("Expected description 'Looking for a drill', got %s", payload.Description)
	}

	if payload.Requester == nil {
		t.Fatal("Expected requester user to be set")
	}

	if payload.Requester.Id != userID {
		t.Errorf("Expected requester user ID %s, got %s", userID, payload.Requester.Id)
	}

	if payload.Requester.Name != "Test User" {
		t.Errorf("Expected requester username 'Test User', got %s", payload.Requester.Name)
	}

	if payload.OfferCount != 0 {
		t.Errorf("Expected offer count 0, got %d", payload.OfferCount)
	}
}

func TestGetFeed_RequestFulfilledStatus(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create fulfilled request
	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Need a hammer", models.RequestState_REQUEST_STATE_FULFILLED)
	createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item, got %d", len(resp.Msg.Items))
	}

	payload := resp.Msg.Items[0].GetRequestCreated()
	if payload == nil {
		t.Fatal("Expected request created payload")
	}

	// Request state is checked on the Request model, not in the feed payload
}

func TestGetFeed_MixedItemTypes(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create gear event (older)
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	gearEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix() - 100,
	}
	_, err := sqlStorage.Insert(context.Background(), gearEvent)
	if err != nil {
		t.Fatalf("Failed to create gear event: %v", err)
	}

	// Create request event (newer)
	requestID := setupTestRequest(t, sqlStorage, userID, communityID, "Looking for tools", models.RequestState_REQUEST_STATE_ACTIVE)
	requestEvent := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		ActorId:           userID,
		Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
		OccurredAtUnixSec: time.Now().Unix(),
	}
	_, err = sqlStorage.Insert(context.Background(), requestEvent)
	if err != nil {
		t.Fatalf("Failed to create request event: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 2 {
		t.Fatalf("Expected 2 feed items, got %d", len(resp.Msg.Items))
	}

	// Verify newest (request) is first
	if resp.Msg.Items[0].ItemType != api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED {
		t.Error("Expected REQUEST_CREATED as first item (newest)")
	}

	// Verify older (gear) is second
	if resp.Msg.Items[1].ItemType != api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
		t.Error("Expected GEAR_SHARED as second item (older)")
	}
}

func TestGetFeed_RequestWithOffers(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	requesterID := setupTestUser(t, sqlStorage, "requester@test.com", "Requester")
	offerer1ID := setupTestUser(t, sqlStorage, "offerer1@test.com", "Offerer 1")
	offerer2ID := setupTestUser(t, sqlStorage, "offerer2@test.com", "Offerer 2")
	communityID := setupTestCommunity(t, sqlStorage, requesterID, "Test Community")

	// Add offerers as members
	for _, offererID := range []string{offerer1ID, offerer2ID} {
		membership := &models.CommunityUser{
			CommunityId:      communityID,
			UserId:           offererID,
			InviterId:        requesterID,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		_, err := sqlStorage.Insert(context.Background(), membership)
		if err != nil {
			t.Fatalf("Failed to add member: %v", err)
		}
	}

	// Create request with offers
	request := &models.Request{
		RequesterId: requesterID,
		Description: "Need help",
		State:       models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
	}
	requestID, err := sqlStorage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Link request to community
	_, err = sqlStorage.Insert(context.Background(), &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	if err != nil {
		t.Fatalf("Failed to link request to community: %v", err)
	}

	// Create offers from offerers
	for _, offererID := range []string{offerer1ID, offerer2ID} {
		offer := &models.RequestOffer{
			RequestId:        requestID,
			UserId:           offererID,
			CommunityId:      communityID,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		_, err = sqlStorage.Insert(context.Background(), offer)
		if err != nil {
			t.Fatalf("Failed to create offer: %v", err)
		}
	}

	createRequestCreatedEvent(t, sqlStorage, communityID, requesterID, requestID)

	ctx := createAuthenticatedContext(requesterID, "requester@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item, got %d", len(resp.Msg.Items))
	}

	payload := resp.Msg.Items[0].GetRequestCreated()
	if payload == nil {
		t.Fatal("Expected request created payload")
	}

	if payload.OfferCount != 2 {
		t.Errorf("Expected offer count 2, got %d", payload.OfferCount)
	}
}

// TestGetFeed_ArchivedRequestExcluded verifies that archived requests (fulfilled/cancelled)
// are excluded from the feed.
func TestGetFeed_ArchivedRequestExcluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create request
	request := &models.Request{
		RequesterId: userID,
		Title:       "Need a drill",
		Description: "Looking for a drill",
		State:       models.RequestState_REQUEST_STATE_FULFILLED,
	}
	requestID, err := sqlStorage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Link request to community with archived=true
	communityRequest := &models.CommunityRequest{
		RequestId:       requestID,
		CommunityId:     communityID,
		Archived:        true, // Mark as archived (fulfilled/cancelled)
		SharedAtUnixSec: time.Now().Unix(),
	}
	_, err = sqlStorage.Insert(context.Background(), communityRequest)
	if err != nil {
		t.Fatalf("Failed to create community request: %v", err)
	}

	// Create request created event
	createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - archived request should not appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Archived request should not appear in feed
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 feed items for archived request, got %d", len(resp.Msg.Items))
	}
}

// TestGetFeed_NonArchivedRequestIncluded verifies that non-archived requests appear in the feed.
func TestGetFeed_NonArchivedRequestIncluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create request
	request := &models.Request{
		RequesterId: userID,
		Title:       "Need a hammer",
		Description: "Looking for a hammer",
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	requestID, err := sqlStorage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Link request to community with archived=false
	communityRequest := &models.CommunityRequest{
		RequestId:       requestID,
		CommunityId:     communityID,
		Archived:        false, // Not archived - should appear in feed
		SharedAtUnixSec: time.Now().Unix(),
	}
	_, err = sqlStorage.Insert(context.Background(), communityRequest)
	if err != nil {
		t.Fatalf("Failed to create community request: %v", err)
	}

	// Create request created event
	eventID := createRequestCreatedEvent(t, sqlStorage, communityID, userID, requestID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - non-archived request should appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Non-archived request should appear in feed
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item for non-archived request, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected item ID %s, got %s", eventID, item.Id)
	}

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_REQUEST_CREATED {
		t.Errorf("Expected REQUEST_CREATED type, got %v", item.ItemType)
	}

	requestPayload := item.GetRequestCreated()
	if requestPayload == nil {
		t.Fatal("Expected request created payload")
	}

	if requestPayload.RequestId != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, requestPayload.RequestId)
	}

	if requestPayload.Title != "Need a hammer" {
		t.Errorf("Expected title 'Need a hammer', got %s", requestPayload.Title)
	}
}

// TestGetFeed_ArchivedExperienceExcluded verifies that archived experiences (completed/cancelled)
// are excluded from the feed.
func TestGetFeed_ArchivedExperienceExcluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	experienceID := setupTestExperience(t, sqlStorage, userID, "Test Experience")

	// Share experience with community with archived=true
	setupCommunityExperience(t, sqlStorage, communityID, experienceID, true)

	// Create experience created event
	createExperienceCreatedEvent(t, sqlStorage, communityID, userID, experienceID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - archived experience should not appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Archived experience should not appear in feed
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 feed items for archived experience, got %d", len(resp.Msg.Items))
	}
}

// TestGetFeed_NonArchivedExperienceIncluded verifies that non-archived experiences appear in the feed.
func TestGetFeed_NonArchivedExperienceIncluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	experienceID := setupTestExperience(t, sqlStorage, userID, "Test Experience")

	// Share experience with community with archived=false
	setupCommunityExperience(t, sqlStorage, communityID, experienceID, false)

	// Create experience created event
	eventID := createExperienceCreatedEvent(t, sqlStorage, communityID, userID, experienceID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - non-archived experience should appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Non-archived experience should appear in feed
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item for non-archived experience, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected item ID %s, got %s", eventID, item.Id)
	}

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_EXPERIENCE_CREATED {
		t.Errorf("Expected EXPERIENCE_CREATED type, got %v", item.ItemType)
	}

	experiencePayload := item.GetExperienceCreated()
	if experiencePayload == nil {
		t.Fatal("Expected experience created payload")
	}

	if experiencePayload.ExperienceId != experienceID {
		t.Errorf("Expected experience ID %s, got %s", experienceID, experiencePayload.ExperienceId)
	}

	if experiencePayload.Name != "Test Experience" {
		t.Errorf("Expected name 'Test Experience', got %s", experiencePayload.Name)
	}
}
