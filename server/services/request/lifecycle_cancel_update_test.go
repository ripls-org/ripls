package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_CancelRequest(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Looking for a power drill",
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Cancel the request
	cancelReq := connect.NewRequest(&api.CancelRequestRequest{
		RequestId: requestID,
	})

	_, err := service.CancelRequest(ctx, cancelReq)
	if err != nil {
		t.Fatalf("CancelRequest failed: %v", err)
	}
	// Note: REQUEST_CANCELLED doesn't trigger notifications

	// Verify request state changed to CANCELLED
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.State != models.RequestState_REQUEST_STATE_CANCELLED {
		t.Errorf("Expected state CANCELLED, got %v", request.State)
	}

	// Verify event was created
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 cancelled event, got %d", len(events))
	}
}

func TestService_UpdateRequest(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	// Create test locations
	location1 := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
		},
		Address: &models.Address{
			Locality:     "New York",
			AddressLines: []string{"123 Main St"},
		},
	}
	locationID1, err := testStorage.Insert(context.Background(), location1)
	if err != nil {
		t.Fatalf("Failed to create location 1: %v", err)
	}

	location2 := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  40.7580,
			LongitudeDeg: -73.9855,
		},
		Address: &models.Address{
			Locality:     "Manhattan",
			AddressLines: []string{"456 Work Ave"},
		},
	}
	locationID2, err := testStorage.Insert(context.Background(), location2)
	if err != nil {
		t.Fatalf("Failed to create location 2: %v", err)
	}

	// Create test media
	media := &models.Media{
		UserId:      requesterID,
		ContentType: "image/jpeg",
		Filename:    proto.String("test.jpg"),
		SizeBytes:   1024,
		StorageUrl:  "https://example.com/test.jpg",
	}
	mediaID, err := testStorage.Insert(context.Background(), media)
	if err != nil {
		t.Fatalf("Failed to create media: %v", err)
	}

	// Create a request
	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createReq := connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Power Drill",
		Description: "Looking for a power drill",
		LocationId:  locationID1,
	})
	createResp, _ := service.SubmitRequest(ctx, createReq)
	services.WaitForStockImagery(t, stockImageryDone) // Wait for async stock imagery fetch

	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	// Update all fields: title, description, media, location
	updateReq := connect.NewRequest(&api.UpdateRequestRequest{
		RequestId:   requestID,
		Title:       "Hammer Drill",
		Description: "Looking for a hammer drill (updated)",
		MediaIds:    []string{mediaID},
		LocationId:  locationID2,
	})

	_, err = service.UpdateRequest(ctx, updateReq)
	if err != nil {
		t.Fatalf("UpdateRequest failed: %v", err)
	}

	// Verify all fields were updated
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), requestID, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.Title != "Hammer Drill" {
		t.Errorf("Expected title 'Hammer Drill', got %s", request.Title)
	}

	if request.Description != "Looking for a hammer drill (updated)" {
		t.Errorf("Expected updated description, got %s", request.Description)
	}

	if len(request.MediaIds) != 1 || request.MediaIds[0] != mediaID {
		t.Errorf("Expected media_ids [%s], got %v", mediaID, request.MediaIds)
	}

	if request.LocationId != locationID2 {
		t.Errorf("Expected location_id %s, got %s", locationID2, request.LocationId)
	}
}
