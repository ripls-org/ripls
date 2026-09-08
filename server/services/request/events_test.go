package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestRecordCommunityEventForRequest verifies that the correct CommunityEventType is
// written for each RequestState and that the returned event ID is non-empty when an
// event is emitted.
//
// Only REQUEST_FULFILLED triggers push notifications (per shouldNotify); the other
// event types (REQUEST_CREATED, REQUEST_CANCELLED) are recorded synchronously and do
// not use the notification-done channel.
func TestRecordCommunityEventForRequest(t *testing.T) {
	tests := []struct {
		name             string
		state            models.RequestState
		wantEventType    models.CommunityEventType
		wantEmitted      bool // false means no DB row should be written
		wantNotification bool // true only when shouldNotify fires for this event type
	}{
		{
			name:             "active emits REQUEST_CREATED",
			state:            models.RequestState_REQUEST_STATE_ACTIVE,
			wantEventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			wantEmitted:      true,
			wantNotification: false,
		},
		{
			name:             "fulfilled emits REQUEST_FULFILLED",
			state:            models.RequestState_REQUEST_STATE_FULFILLED,
			wantEventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
			wantEmitted:      true,
			wantNotification: true,
		},
		{
			name:             "cancelled emits REQUEST_CANCELLED",
			state:            models.RequestState_REQUEST_STATE_CANCELLED,
			wantEventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
			wantEmitted:      true,
			wantNotification: false,
		},
		{
			name:        "offers_received emits no event",
			state:       models.RequestState_REQUEST_STATE_OFFERS_RECEIVED,
			wantEmitted: false,
		},
		{
			name:        "unspecified state emits no event",
			state:       models.RequestState_REQUEST_STATE_UNSPECIFIED,
			wantEmitted: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

			requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
			communityID := setupCommunityWithMembers(t, testStorage, requesterID)

			// Create a request so we have a valid request ID.
			ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
			createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
				Description: "Test request for event recording",
			}))
			if err != nil {
				t.Fatalf("SubmitRequest failed: %v", err)
			}
			services.WaitForStockImagery(t, stockImageryDone)
			requestID := createResp.Msg.RequestId
			shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

			// Drain the REQUEST_CREATED event that SubmitRequest writes.
			events, _ := testStorage.QueryByFields(context.Background(), map[string]any{
				"community_id": communityID,
				"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			}, &models.CommunityEvent{})
			existingCount := len(events)

			eventID, err := service.recordCommunityEventForRequest(
				ctx, requestID, tc.state, requesterID, communityID, "", nil,
			)
			if err != nil {
				t.Fatalf("recordCommunityEventForRequest returned error: %v", err)
			}

			if tc.wantEmitted {
				if eventID == "" {
					t.Error("expected non-empty event ID, got empty string")
				}

				// Only wait for the notification signal when this event type fires one.
				if tc.wantNotification {
					services.WaitForNotification(t, notifDone)
				}

				rows, queryErr := testStorage.QueryByFields(context.Background(), map[string]any{
					"community_id": communityID,
					"event_type":   tc.wantEventType,
				}, &models.CommunityEvent{})
				if queryErr != nil {
					t.Fatalf("failed to query events: %v", queryErr)
				}

				// Subtract existing REQUEST_CREATED rows when checking for a new ACTIVE row.
				wantCount := 1
				if tc.wantEventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED {
					wantCount = existingCount + 1
				}
				if len(rows) != wantCount {
					t.Errorf("expected %d event(s) of type %v, got %d", wantCount, tc.wantEventType, len(rows))
				}
			} else if eventID != "" {
				t.Errorf("expected empty event ID for state %v, got %q", tc.state, eventID)
			}
		})
	}
}

// TestRecordOfferMadeEvent verifies that a REQUEST_OFFER_MADE event is written.
func TestRecordOfferMadeEvent(t *testing.T) {
	service, testStorage, _, notifDone, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	if err := service.recordOfferMadeEvent(offererCtx, requestID, offererID, communityID); err != nil {
		t.Fatalf("recordOfferMadeEvent returned error: %v", err)
	}
	services.WaitForNotification(t, notifDone)

	rows, queryErr := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
	}, &models.CommunityEvent{})
	if queryErr != nil {
		t.Fatalf("failed to query events: %v", queryErr)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 REQUEST_OFFER_MADE event, got %d", len(rows))
	}
	event := rows[0].(*models.CommunityEvent)
	if event.ActorId != offererID {
		t.Errorf("expected actor_id %s, got %s", offererID, event.ActorId)
	}
	if event.GetRequestId() != requestID {
		t.Errorf("expected request_id %s, got %s", requestID, event.GetRequestId())
	}
}

// TestRecordOfferWithdrawnEvent verifies that a REQUEST_OFFER_WITHDRAWN event is written.
// REQUEST_OFFER_WITHDRAWN does not trigger push notifications, so there is no notification
// channel signal to wait for.
func TestRecordOfferWithdrawnEvent(t *testing.T) {
	service, testStorage, _, _, stockImageryDone := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "requester@example.com")
	offererID := setupTestUser(t, testStorage, "Offerer", "offerer@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID, offererID)

	ctx := createAuthenticatedContext(requesterID, "requester@example.com", models.Role_ROLE_USER)
	createResp, err := service.SubmitRequest(ctx, connect.NewRequest(&api.SubmitRequestRequest{
		Description: "Need a ladder",
	}))
	if err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	services.WaitForStockImagery(t, stockImageryDone)
	requestID := createResp.Msg.RequestId
	shareRequestInto(t, ctx, service, createResp.Msg.RequestId, communityID)

	offererCtx := createAuthenticatedContext(offererID, "offerer@example.com", models.Role_ROLE_USER)
	if err := service.recordOfferWithdrawnEvent(offererCtx, requestID, offererID, communityID); err != nil {
		t.Fatalf("recordOfferWithdrawnEvent returned error: %v", err)
	}

	rows, queryErr := testStorage.QueryByFields(context.Background(), map[string]any{
		"community_id": communityID,
		"event_type":   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
	}, &models.CommunityEvent{})
	if queryErr != nil {
		t.Fatalf("failed to query events: %v", queryErr)
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 REQUEST_OFFER_WITHDRAWN event, got %d", len(rows))
	}
	event := rows[0].(*models.CommunityEvent)
	if event.ActorId != offererID {
		t.Errorf("expected actor_id %s, got %s", offererID, event.ActorId)
	}
	if event.GetRequestId() != requestID {
		t.Errorf("expected request_id %s, got %s", requestID, event.GetRequestId())
	}
}
