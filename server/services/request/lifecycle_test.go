package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// shareRequestInto shares requestID into communityID — the post-#2529
// equivalent of the removed SubmitRequestRequest.community_id. SubmitRequest now
// only creates the request in its own per-item community, so tests that need the
// request in a specific community share it explicitly.
func TestService_SubmitRequest(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

	// Create a test location
	location := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  40.7128,
			LongitudeDeg: -74.0060,
		},
		Address: &models.Address{
			Locality:     "New York",
			AddressLines: []string{"123 Main St"},
		},
	}
	locationID, err := testStorage.Insert(context.Background(), location)
	if err != nil {
		t.Fatalf("Failed to create location: %v", err)
	}

	req := connect.NewRequest(&api.SubmitRequestRequest{
		Title:       "Power Drill",
		Description: "Looking for a power drill for weekend project",
		LocationId:  locationID,
	})

	resp, err := service.SubmitRequest(ctx, req)
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)
	// Wait for async stock imagery fetch to complete before test ends
	services.WaitForStockImagery(t, stockImageryDone)

	if resp.Msg.RequestId == "" {
		t.Error("Expected non-empty request ID")
	}

	// Verify request was created correctly
	request := &models.Request{}
	err = testStorage.GetByID(context.Background(), resp.Msg.RequestId, request)
	if err != nil {
		t.Fatalf("Failed to retrieve request: %v", err)
	}

	if request.RequesterId != requesterID {
		t.Errorf("Expected requester_id %s, got %s", requesterID, request.RequesterId)
	}

	if request.Title != "Power Drill" {
		t.Errorf("Expected title 'Power Drill', got %s", request.Title)
	}

	if request.Description != "Looking for a power drill for weekend project" {
		t.Errorf("Expected description 'Looking for a power drill for weekend project', got %s", request.Description)
	}

	if request.LocationId != locationID {
		t.Errorf("Expected location_id %s, got %s", locationID, request.LocationId)
	}

	if request.State != models.RequestState_REQUEST_STATE_ACTIVE {
		t.Errorf("Expected state ACTIVE, got %v", request.State)
	}

	// Verify ImpactEstimate was persisted at creation
	if request.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set after SubmitRequest")
	} else if request.ImpactEstimate.TimeSaved == nil || request.ImpactEstimate.TimeSaved.Minutes == nil {
		t.Error("Expected TimeSaved.Minutes to be set after SubmitRequest")
	}

	// Verify provenance on TimeSaved
	if ts := request.ImpactEstimate.GetTimeSaved(); ts != nil {
		if ts.Provenance == nil {
			t.Error("Expected TimeSaved.Provenance to be set after SubmitRequest")
		} else {
			if ts.Provenance.Source != models.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
				t.Errorf("Expected TimeSaved source CONFIG_DEFAULT, got %v", ts.Provenance.Source)
			}
			if ts.Provenance.Name == "" {
				t.Error("Expected TimeSaved provenance name to be non-empty")
			}
			if ts.Provenance.Version <= 0 {
				t.Errorf("Expected TimeSaved provenance version > 0, got %d", ts.Provenance.Version)
			}
		}
	}

	// Verify community event was created
	events, err := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
	}, &models.CommunityEvent{})
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

func TestService_SubmitRequest_EmitsRequestCreatedSystemMessage(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		wantAction  models.ChatSystemAction
	}{
		{
			name:        "emits REQUEST_CREATED with title in description",
			title:       "Power Drill",
			description: "Need a power drill for the weekend",
			wantAction:  models.ChatSystemAction_CHAT_SYSTEM_ACTION_REQUEST_CREATED,
		},
		{
			name:        "emits REQUEST_CREATED even with empty title",
			title:       "",
			description: "Need help with moving",
			wantAction:  models.ChatSystemAction_CHAT_SYSTEM_ACTION_REQUEST_CREATED,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

			requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
			communityID := setupCommunityWithMembers(t, testStorage, requesterID)

			ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)

			req := connect.NewRequest(&api.SubmitRequestRequest{
				Title:       tc.title,
				Description: tc.description,
			})

			resp, err := service.SubmitRequest(ctx, req)
			if err != nil {
				t.Fatalf("SubmitRequest failed: %v", err)
			}
			services.WaitForStockImagery(t, stockImageryDone)

			requestID := resp.Msg.RequestId
			shareRequestInto(t, ctx, service, resp.Msg.RequestId, communityID)

			// Find the conversation for this request (canonical location: Request.conversation_id).
			requestStored := &models.Request{}
			if err := testStorage.GetByID(context.Background(), requestID, requestStored); err != nil {
				t.Fatalf("Failed to find request: %v", err)
			}
			conversationID := requestStored.ConversationId

			// Query system messages for the conversation
			messages, err := testStorage.QueryByField(context.Background(), "conversation_id", conversationID, &models.ChatMessage{})
			if err != nil {
				t.Fatalf("Failed to query messages: %v", err)
			}

			// Find the REQUEST_CREATED system message
			var found bool
			for _, m := range messages {
				msg := m.(*models.ChatMessage)
				if sm := msg.GetSystemMessage(); sm != nil && sm.Action == tc.wantAction {
					found = true
					if sm.Description == "" {
						t.Error("Expected non-empty description for REQUEST_CREATED message")
					}
					break
				}
			}

			if !found {
				t.Errorf("Expected REQUEST_CREATED system message in conversation %s, got %d messages", conversationID, len(messages))
			}
		})
	}
}
